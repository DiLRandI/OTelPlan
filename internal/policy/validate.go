package policy

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

var ruleIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// Validate checks policy schema, selectors, and safety without modifying plan.
// Rules and exclusions are checked in declaration order; symbols are not resolved.
func Validate(plan *model.Policy) model.DiagnosticErrorList {
	var diags model.DiagnosticErrorList

	if plan == nil {
		return model.DiagnosticErrorList{policyDiagnostic(model.CodeInvalidPolicy, "policy is required", "")}
	}

	diags = append(diags, validatePolicySettings(plan)...)

	seen := make(map[string]bool, len(plan.Rules))

	for index := range plan.Rules {
		diags = append(diags, validateRule(&plan.Rules[index])...)

		diags = append(diags, validateMatch(plan.Rules[index].ID, plan.Rules[index].Match)...)

		if plan.Rules[index].Exclude != nil {
			diags = append(diags, validateMatch(plan.Rules[index].ID, *plan.Rules[index].Exclude)...)
		}

		ruleID := plan.Rules[index].ID
		if ruleID == "" {
			diags = append(diags, policyDiagnostic(
				model.CodeInvalidPolicy,
				fmt.Sprintf("rules[%d]: id is required", index),
				"",
			))

			continue
		}

		if !ruleIDPattern.MatchString(ruleID) {
			diags = append(diags, policyDiagnostic(
				model.CodeInvalidPolicy,
				fmt.Sprintf("rule %q: id must match %s", ruleID, ruleIDPattern.String()),
				"",
			))
		}

		if seen[ruleID] {
			diags = append(diags, policyDiagnostic(
				model.CodeConflictingRules,
				fmt.Sprintf("duplicate rule id %q", ruleID),
				"",
			))
		}

		seen[ruleID] = true
	}

	diags = append(diags, validateExclusions(plan.Exclusions, seen)...)

	return diags
}

func validateExclusions(exclusions []model.Exclusion, seen map[string]bool) model.DiagnosticErrorList {
	var diags model.DiagnosticErrorList

	for index := range exclusions {
		exclusion := &exclusions[index]

		diags = append(diags, validateMatch(exclusion.ID, exclusion.Match)...)

		if seen[exclusion.ID] || (exclusion.ID != "" && !ruleIDPattern.MatchString(exclusion.ID)) {
			diags = append(diags, policyDiagnostic(
				model.CodeInvalidPolicy,
				"exclusion ID must be valid and unique across rules and exclusions",
				exclusion.ID,
			))
		}

		seen[exclusion.ID] = true

		if exclusion.ID == "" {
			diags = append(diags, policyDiagnostic(
				model.CodeInvalidPolicy,
				fmt.Sprintf("exclusions[%d]: id is required", index),
				"",
			))
		}

		if exclusion.Match.IsEmpty() {
			diags = append(diags, policyDiagnostic(
				model.CodeInvalidSelector,
				fmt.Sprintf("exclusion %q: match must select at least one field", exclusion.ID),
				"",
			))
		}
	}

	return diags
}

func validatePolicySettings(plan *model.Policy) model.DiagnosticErrorList {
	var diags model.DiagnosticErrorList

	diags = append(diags, validateDefaults(plan.Defaults)...)

	if plan.Backend.Name != "" && plan.Backend.Name != model.BackendNameOTelC {
		diags = append(diags, policyDiagnostic(
			model.CodeInvalidPolicy,
			"unsupported backend",
			"",
		))
	}

	if plan.APIVersion != model.APIVersionV1Alpha1 {
		diags = append(diags, policyDiagnostic(
			model.CodeInvalidPolicy,
			fmt.Sprintf("apiVersion %q is not supported, want %q", plan.APIVersion, model.APIVersionV1Alpha1),
			"",
		))
	}

	if plan.Kind != model.KindInstrumentationPlan {
		diags = append(diags, policyDiagnostic(
			model.CodeInvalidPolicy,
			fmt.Sprintf("kind %q is not supported, want %q", plan.Kind, model.KindInstrumentationPlan),
			"",
		))
	}

	if plan.Backend.Name == "" {
		diags = append(diags, policyDiagnostic(
			model.CodeInvalidPolicy,
			"backend.name is required",
			"",
		))
	}

	if plan.Backend.Version == "" {
		diags = append(diags, policyDiagnostic(
			model.CodeBackendVersionMismatch,
			"backend.version must be pinned to an exact version for lock/build workflows",
			"",
		))
	}

	if len(plan.Rules) == 0 {
		diags = append(diags, policyDiagnostic(
			model.CodeInvalidPolicy,
			"at least one rule is required",
			"",
		))
	}

	return diags
}

func validateDefaults(defaults model.Defaults) model.DiagnosticErrorList {
	var diags model.DiagnosticErrorList

	if defaults.Attributes.Arguments || defaults.Attributes.Results {
		diags = append(diags, policyDiagnostic(
			model.CodeCaptureNotAllowed,
			"blanket argument/result capture is prohibited; use explicit attribute rules",
			"",
		))
	}

	switch defaults.Context.Mode {
	case "", model.ContextModeRequire, model.ContextModeRoot:
	default:
		diags = append(diags, policyDiagnostic(
			model.CodeInvalidPolicy,
			"unsupported default context mode",
			"",
		))
	}

	err := ValidateTemplate(defaults.SpanName)
	if err != nil {
		diags = append(diags, policyDiagnostic(
			model.CodeUnknownTemplateVar,
			"defaults.spanName: "+err.Error(),
			"",
		))
	}

	return diags
}

func validateRule(rule *model.Rule) model.DiagnosticErrorList {
	var diags model.DiagnosticErrorList

	if rule.Match.IsEmpty() {
		diags = append(diags, policyDiagnostic(
			model.CodeInvalidSelector,
			fmt.Sprintf("rule %q: match must select at least one field", rule.ID),
			"",
		))
	}

	if rule.Exclude != nil && rule.Exclude.IsEmpty() {
		diags = append(diags, policyDiagnostic(
			model.CodeInvalidSelector,
			fmt.Sprintf("rule %q: exclude must select at least one field", rule.ID),
			"",
		))
	}

	diags = append(diags, validateRuleSpan(rule)...)

	keys := map[string]bool{}
	for index := range rule.Attributes {
		if keys[rule.Attributes[index].Key] {
			diags = append(diags, policyDiagnostic(
				model.CodeInvalidPolicy,
				"duplicate attribute key",
				rule.ID,
			))
		}

		keys[rule.Attributes[index].Key] = true

		diags = append(diags, validateAttribute(rule.ID, index, &rule.Attributes[index])...)
	}

	return diags
}

func validateRuleSpan(rule *model.Rule) model.DiagnosticErrorList {
	var diags model.DiagnosticErrorList

	if rule.Span != nil && rule.Span.Kind != "" && rule.Span.Kind != "internal" {
		diags = append(diags, policyDiagnostic(
			model.CodeInvalidPolicy,
			fmt.Sprintf("rule %q: span.kind %q is not supported, only \"internal\"", rule.ID, rule.Span.Kind),
			"",
		))
	}

	if rule.Span != nil && rule.Span.Name != "" {
		err := ValidateTemplate(rule.Span.Name)
		if err != nil {
			diags = append(diags, policyDiagnostic(
				model.CodeUnknownTemplateVar,
				fmt.Sprintf("rule %q: span.name: %v", rule.ID, err),
				"",
			))
		}
	}

	return diags
}

func validateAttribute(ruleID string, idx int, attr *model.AttributeRule) model.DiagnosticErrorList {
	var diags model.DiagnosticErrorList

	loc := fmt.Sprintf("rule %q attributes[%d]", ruleID, idx)

	if attr.Key == "" {
		diags = append(diags, policyDiagnostic(
			model.CodeInvalidPolicy,
			loc+": key is required",
			"",
		))
	}

	if attr.Key != "" && hasReservedPrefix(attr.Key) {
		diags = append(diags, policyDiagnostic(
			model.CodeInvalidPolicy,
			fmt.Sprintf("%s: key %q uses the reserved otel. prefix", loc, attr.Key),
			"",
		))
	}

	diags = append(diags, validateAttributeSource(loc, attr.From)...)

	if attr.Safety != nil {
		switch attr.Safety.Classification {
		case "", model.ClassificationPublic, model.ClassificationInternal,
			model.ClassificationPII, model.ClassificationSecret:
		default:
			diags = append(diags, policyDiagnostic(
				model.CodeInvalidPolicy,
				fmt.Sprintf("%s: unknown safety classification %q", loc, attr.Safety.Classification),
				"",
			))
		}
	}

	return diags
}

func validateAttributeSource(loc string, src model.AttributeSource) model.DiagnosticErrorList {
	var diags model.DiagnosticErrorList

	set := 0

	for _, v := range []bool{src.Argument != "", src.Result != "", src.Constant != nil} {
		if v {
			set++
		}
	}

	if set == 0 {
		diags = append(diags, policyDiagnostic(
			model.CodeInvalidPolicy,
			loc+": from must set exactly one of argument, result, constant",
			"",
		))
	}

	if set > 1 {
		diags = append(diags, policyDiagnostic(
			model.CodeInvalidPolicy,
			loc+": from must not set more than one source",
			"",
		))
	}

	return diags
}

func hasReservedPrefix(key string) bool {
	return strings.HasPrefix(strings.ToLower(key), "otel.")
}

func validateMatch(ruleID string, match model.Match) model.DiagnosticErrorList {
	var diags model.DiagnosticErrorList

	switch match.Ownership {
	case "", model.OwnershipApplication, model.OwnershipDependency, model.OwnershipAny:
	default:
		diags = append(diags, policyDiagnostic(
			model.CodeInvalidSelector,
			"unknown ownership selector",
			ruleID,
		))
	}

	for _, list := range [][]string{
		match.Packages, match.Files, match.Symbols, match.Functions,
		match.Receivers, match.Methods, match.Implements,
	} {
		for _, item := range list {
			if strings.TrimSpace(item) == "" {
				diags = append(diags, policyDiagnostic(
					model.CodeInvalidSelector,
					"selector values must not be empty",
					ruleID,
				))
			}
		}
	}

	return diags
}

func policyDiagnostic(code model.Code, message, ruleID string) model.DiagnosticError {
	return model.DiagnosticError{
		Severity: model.SeverityError,
		Code:     code,
		Message:  message,
		RuleID:   ruleID,
		Symbol:   "",
		File:     "",
		Line:     0,
	}
}
