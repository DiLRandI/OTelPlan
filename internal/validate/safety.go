// Package validate checks resolved instrumentation targets and attribute access
// against the discovered code model before backend generation.
package validate

import (
	"slices"
	"strings"
	"unicode"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

// Options controls sensitive-name detection and plan-size limits.
type Options struct {
	// DenyPatterns supplements the built-in sensitive-name patterns.
	DenyPatterns []string
	// AllowLargePlan acknowledges exceeding MaximumTargets; warnings still apply.
	AllowLargePlan bool
	// WarningTargets defaults to 100 when non-positive.
	WarningTargets int
	// MaximumTargets defaults to 2000 when non-positive.
	MaximumTargets int
}

// Safety reports unresolved targets, unsafe attributes, and potential span noise.
// It also warns whenever a plan exceeds 500 targets, independently of Options.
// Attribute values are not included in diagnostics.
func Safety(code *model.CodeModel, plan model.ResolvedPlan, opts Options) model.DiagnosticErrorList {
	var diags model.DiagnosticErrorList

	if code == nil {
		return model.DiagnosticErrorList{{Severity: model.SeverityError, Code: model.CodeUnresolvedSymbol, Message: "code model is required"}}
	}

	if opts.WarningTargets <= 0 {
		opts.WarningTargets = 100
	}

	if opts.MaximumTargets <= 0 {
		opts.MaximumTargets = 2000
	}

	if len(plan.Targets) > opts.WarningTargets {
		diags = append(diags, model.DiagnosticError{Severity: model.SeverityWarning, Code: model.CodeBroadPlan, Message: "policy selects many targets; review trace volume"})
	}

	if len(plan.Targets) > 500 {
		diags = append(diags, model.DiagnosticError{Severity: model.SeverityWarning, Code: model.CodeBroadPlan, Message: "policy selects more than 500 targets; review span noise and telemetry cost carefully"})
	}

	if len(plan.Targets) > opts.MaximumTargets && !opts.AllowLargePlan {
		diags = append(diags, model.DiagnosticError{Severity: model.SeverityError, Code: model.CodeBroadPlan, Message: "plan exceeds target limit; explicit large-plan acknowledgment is required"})
	}

	secrets := append([]string{"password", "passwd", "pwd", "secret", "token", "authorization", "cookie", "apikey", "privatekey", "credential", "session"}, opts.DenyPatterns...)

	for _, target := range plan.Targets {
		add := func(severity model.Severity, code model.Code, message string) {
			diags = append(diags, model.DiagnosticError{Severity: severity, Code: code, RuleID: target.RuleID, Symbol: target.SymbolID, Message: message})
		}

		symbol, ok := code.Symbol(target.SymbolID)

		if !ok {
			add(model.SeverityError, model.CodeUnresolvedSymbol, "selected symbol is absent from the code model")

			continue
		}

		if symbol.Generated || symbol.TestFile {
			add(model.SeverityWarning, model.CodeBroadPlan, "selection includes generated or test code")
		}

		if target.ContextStrategy.Strategy == model.ContextStrategyRoot {
			add(model.SeverityWarning, model.CodeMissingContext, "explicit root span will not inherit caller context")
		}

		if target.ErrorStrategy.Record && len(target.ErrorStrategy.Indexes) == 0 {
			add(model.SeverityWarning, model.CodeInvalidPolicy, "error recording requested but target has no error result")
		}

		for _, attr := range target.Attributes {
			if _, err := AttributeAccessor(code, *symbol, attr.From); err != nil {
				add(model.SeverityError, model.CodeCaptureNotAllowed, err.Error())
			}

			source := attr.Key + " " + attr.From.Argument + " " + attr.From.Result
			if matchesAny(source, secrets) || attr.Classification == model.ClassificationSecret {
				if !attr.Allow || attr.Classification != model.ClassificationSecret {
					add(model.SeverityError, model.CodeSecretAttribute, "sensitive attribute requires explicit secret classification and acknowledgment")
				}
			}

			if matchesAny(source, []string{"email", "phone", "address", "name", "userid", "customerid", "ipaddress", "deviceid"}) || hasIPToken(source) || attr.Classification == model.ClassificationPII {
				if !attr.Allow || (attr.Classification != model.ClassificationPII && attr.Classification != model.ClassificationSecret) {
					add(model.SeverityWarning, model.CodePIIAttribute, "attribute may contain personal information; review classification and consent")
				}
			}

			if matchesAny(source, []string{"url", "query", "body", "uuid", "orderid", "userid", "timestamp", "message"}) {
				add(model.SeverityWarning, model.CodeCardinalityWarning, "attribute may have unbounded cardinality; prefer a bounded category")
			}
		}
	}

	return diags
}

func matchesAny(value string, patterns []string) bool {
	normalize := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == '_' || r == '-' || r == '.' {
				return -1
			}

			return r
		}, strings.ToLower(s))
	}

	value = normalize(value)

	for _, pattern := range patterns {
		normalized := normalize(pattern)
		if normalized != "" && strings.Contains(value, normalized) {
			return true
		}
	}

	return false
}

func hasIPToken(value string) bool {
	return slices.Contains(strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), "ip")
}
