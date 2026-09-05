package policy

import (
	"fmt"
	"regexp"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

var ruleIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func Validate(p *model.Policy) model.DiagnosticList {
	var diags model.DiagnosticList

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
	for i := range r.Attributes {
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
	return len(key) >= 5 && (key[:5] == "otel." || key[:5] == "OTEL.")
}
