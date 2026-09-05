package policy

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

var ruleIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func Validate(p *model.Policy) model.DiagnosticList {
	var diags model.DiagnosticList

	if p == nil {
		return model.DiagnosticList{{Severity: model.SeverityError, Code: model.CodeInvalidPolicy, Message: "policy is required"}}
	}
	if p.Defaults.Attributes.Arguments || p.Defaults.Attributes.Results {
		diags = append(diags, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeCaptureNotAllowed, Message: "blanket argument/result capture is prohibited; use explicit attribute rules"})
	}
	if p.Defaults.Context.Mode != "" && p.Defaults.Context.Mode != model.ContextModeRequire && p.Defaults.Context.Mode != model.ContextModeRoot {
		diags = append(diags, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeInvalidPolicy, Message: "unsupported default context mode"})
	}
	if err := ValidateTemplate(p.Defaults.SpanName); err != nil {
		diags = append(diags, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeUnknownTemplateVar, Message: "defaults.spanName: " + err.Error()})
	}
	if p.Backend.Name != "" && p.Backend.Name != model.BackendNameOTelC {
		diags = append(diags, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeInvalidPolicy, Message: "unsupported backend"})
	}

	if p.APIVersion != model.APIVersionV1Alpha1 {
		diags = append(diags, model.Diagnostic{
			Severity: model.SeverityError,
			Code:     model.CodeInvalidPolicy,
			Message:  fmt.Sprintf("apiVersion %q is not supported, want %q", p.APIVersion, model.APIVersionV1Alpha1),
		})
	}
	if p.Kind != model.KindInstrumentationPlan {
		diags = append(diags, model.Diagnostic{
			Severity: model.SeverityError,
			Code:     model.CodeInvalidPolicy,
			Message:  fmt.Sprintf("kind %q is not supported, want %q", p.Kind, model.KindInstrumentationPlan),
		})
	}
	if p.Backend.Name == "" {
		diags = append(diags, model.Diagnostic{
			Severity: model.SeverityError,
			Code:     model.CodeInvalidPolicy,
			Message:  "backend.name is required",
		})
	}
	if p.Backend.Version == "" {
		diags = append(diags, model.Diagnostic{
			Severity: model.SeverityError,
			Code:     model.CodeBackendVersionMismatch,
			Message:  "backend.version must be pinned to an exact version for lock/build workflows",
		})
	}
	if len(p.Rules) == 0 {
		diags = append(diags, model.Diagnostic{
			Severity: model.SeverityError,
			Code:     model.CodeInvalidPolicy,
			Message:  "at least one rule is required",
		})
	}

	seen := make(map[string]bool, len(p.Rules))
	for i := range p.Rules {
		diags = append(diags, validateRule(&p.Rules[i])...)
		diags = append(diags, validateMatch(p.Rules[i].ID, p.Rules[i].Match)...)
		if p.Rules[i].Exclude != nil {
			diags = append(diags, validateMatch(p.Rules[i].ID, *p.Rules[i].Exclude)...)
		}
		id := p.Rules[i].ID
		if id == "" {
			diags = append(diags, model.Diagnostic{
				Severity: model.SeverityError,
				Code:     model.CodeInvalidPolicy,
				Message:  fmt.Sprintf("rules[%d]: id is required", i),
			})
			continue
		}
		if !ruleIDPattern.MatchString(id) {
			diags = append(diags, model.Diagnostic{
				Severity: model.SeverityError,
				Code:     model.CodeInvalidPolicy,
				Message:  fmt.Sprintf("rule %q: id must match %s", id, ruleIDPattern.String()),
			})
		}
		if seen[id] {
			diags = append(diags, model.Diagnostic{
				Severity: model.SeverityError,
				Code:     model.CodeConflictingRules,
				Message:  fmt.Sprintf("duplicate rule id %q", id),
			})
		}
		seen[id] = true
	}

	for i := range p.Exclusions {
		e := &p.Exclusions[i]
		diags = append(diags, validateMatch(e.ID, e.Match)...)
		if seen[e.ID] || (e.ID != "" && !ruleIDPattern.MatchString(e.ID)) {
			diags = append(diags, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeInvalidPolicy, RuleID: e.ID, Message: "exclusion ID must be valid and unique across rules and exclusions"})
		}
		seen[e.ID] = true
		if e.ID == "" {
			diags = append(diags, model.Diagnostic{
				Severity: model.SeverityError,
				Code:     model.CodeInvalidPolicy,
				Message:  fmt.Sprintf("exclusions[%d]: id is required", i),
			})
		}
		if e.Match.IsEmpty() {
			diags = append(diags, model.Diagnostic{
				Severity: model.SeverityError,
				Code:     model.CodeInvalidSelector,
				Message:  fmt.Sprintf("exclusion %q: match must select at least one field", e.ID),
			})
		}
	}

	return diags
}

func validateRule(r *model.Rule) model.DiagnosticList {
	var diags model.DiagnosticList

	if r.Match.IsEmpty() {
		diags = append(diags, model.Diagnostic{
			Severity: model.SeverityError,
			Code:     model.CodeInvalidSelector,
			Message:  fmt.Sprintf("rule %q: match must select at least one field", r.ID),
		})
	}
	if r.Exclude != nil && r.Exclude.IsEmpty() {
		diags = append(diags, model.Diagnostic{
			Severity: model.SeverityError,
			Code:     model.CodeInvalidSelector,
			Message:  fmt.Sprintf("rule %q: exclude must select at least one field", r.ID),
		})
	}
	if r.Span != nil && r.Span.Kind != "" && r.Span.Kind != "internal" {
		diags = append(diags, model.Diagnostic{
			Severity: model.SeverityError,
			Code:     model.CodeInvalidPolicy,
			Message:  fmt.Sprintf("rule %q: span.kind %q is not supported, only \"internal\"", r.ID, r.Span.Kind),
		})
	}
	if r.Span != nil && r.Span.Name != "" {
		if err := ValidateTemplate(r.Span.Name); err != nil {
			diags = append(diags, model.Diagnostic{
				Severity: model.SeverityError,
				Code:     model.CodeUnknownTemplateVar,
				Message:  fmt.Sprintf("rule %q: span.name: %v", r.ID, err),
			})
		}
	}
	keys := map[string]bool{}
	for i := range r.Attributes {
		if keys[r.Attributes[i].Key] {
			diags = append(diags, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeInvalidPolicy, RuleID: r.ID, Message: "duplicate attribute key"})
		}
		keys[r.Attributes[i].Key] = true
		diags = append(diags, validateAttribute(r.ID, i, &r.Attributes[i])...)
	}

	return diags
}

func validateAttribute(ruleID string, idx int, attr *model.AttributeRule) model.DiagnosticList {
	var diags model.DiagnosticList
	loc := fmt.Sprintf("rule %q attributes[%d]", ruleID, idx)

	if attr.Key == "" {
		diags = append(diags, model.Diagnostic{
			Severity: model.SeverityError,
			Code:     model.CodeInvalidPolicy,
			Message:  loc + ": key is required",
		})
	}
	if attr.Key != "" && hasReservedPrefix(attr.Key) {
		diags = append(diags, model.Diagnostic{
			Severity: model.SeverityError,
			Code:     model.CodeInvalidPolicy,
			Message:  fmt.Sprintf("%s: key %q uses the reserved otel. prefix", loc, attr.Key),
		})
	}

	src := attr.From
	set := 0
	for _, v := range []bool{src.Argument != "", src.Result != "", src.Constant != nil} {
		if v {
			set++
		}
	}
	if set == 0 {
		diags = append(diags, model.Diagnostic{
			Severity: model.SeverityError,
			Code:     model.CodeInvalidPolicy,
			Message:  loc + ": from must set exactly one of argument, result, constant",
		})
	}
	if set > 1 {
		diags = append(diags, model.Diagnostic{
			Severity: model.SeverityError,
			Code:     model.CodeInvalidPolicy,
			Message:  loc + ": from must not set more than one source",
		})
	}
	if attr.Safety != nil {
		switch attr.Safety.Classification {
		case "", model.ClassificationPublic, model.ClassificationInternal, model.ClassificationPII, model.ClassificationSecret:
		default:
			diags = append(diags, model.Diagnostic{
				Severity: model.SeverityError,
				Code:     model.CodeInvalidPolicy,
				Message:  fmt.Sprintf("%s: unknown safety classification %q", loc, attr.Safety.Classification),
			})
		}
	}

	return diags
}

func hasReservedPrefix(key string) bool {
	return strings.HasPrefix(strings.ToLower(key), "otel.")
}

func validateMatch(ruleID string, match model.Match) model.DiagnosticList {
	var diags model.DiagnosticList
	if match.Ownership != "" && match.Ownership != model.OwnershipApplication && match.Ownership != model.OwnershipDependency && match.Ownership != model.OwnershipAny {
		diags = append(diags, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeInvalidSelector, RuleID: ruleID, Message: "unknown ownership selector"})
	}
	for _, list := range [][]string{match.Packages, match.Files, match.Symbols, match.Functions, match.Receivers, match.Methods, match.Implements} {
		for _, item := range list {
			if strings.TrimSpace(item) == "" {
				diags = append(diags, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeInvalidSelector, RuleID: ruleID, Message: "selector values must not be empty"})
			}
		}
	}
	return diags
}
