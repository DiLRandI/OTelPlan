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

const (
	defaultWarningTargets   = 100
	defaultMaximumTargets   = 2000
	spanNoiseWarningTargets = 500
)

// Safety reports unresolved targets, unsafe attributes, and potential span noise.
// It also warns whenever a plan exceeds 500 targets, independently of Options.
// Attribute values are not included in diagnostics.
func Safety(code *model.CodeModel, plan model.ResolvedPlan, opts Options) model.DiagnosticErrorList {
	if code == nil {
		return model.DiagnosticErrorList{safetyDiagnostic(
			model.SeverityError, model.CodeUnresolvedSymbol, "code model is required",
		)}
	}

	diags := planSizeDiagnostics(len(plan.Targets), opts)
	secrets := append([]string{
		"password", "passwd", "pwd", "secret", "token", "authorization",
		"cookie", "apikey", "privatekey", "credential", "session",
	}, opts.DenyPatterns...)

	for _, target := range plan.Targets {
		targetDiags := targetSafety(code, target, secrets)
		for index := range targetDiags {
			targetDiags[index].RuleID = target.RuleID
			targetDiags[index].Symbol = target.SymbolID
		}

		diags = append(diags, targetDiags...)
	}

	return diags
}

func planSizeDiagnostics(count int, opts Options) model.DiagnosticErrorList {
	var diags model.DiagnosticErrorList

	if opts.WarningTargets <= 0 {
		opts.WarningTargets = defaultWarningTargets
	}

	if opts.MaximumTargets <= 0 {
		opts.MaximumTargets = defaultMaximumTargets
	}

	if count > opts.WarningTargets {
		diags = append(diags, safetyDiagnostic(
			model.SeverityWarning, model.CodeBroadPlan,
			"policy selects many targets; review trace volume",
		))
	}

	if count > spanNoiseWarningTargets {
		diags = append(diags, safetyDiagnostic(
			model.SeverityWarning, model.CodeBroadPlan,
			"policy selects more than 500 targets; review span noise and telemetry cost carefully",
		))
	}

	if count > opts.MaximumTargets && !opts.AllowLargePlan {
		diags = append(diags, safetyDiagnostic(
			model.SeverityError, model.CodeBroadPlan,
			"plan exceeds target limit; explicit large-plan acknowledgment is required",
		))
	}

	return diags
}

func targetSafety(code *model.CodeModel, target model.ResolvedTarget, secrets []string) model.DiagnosticErrorList {
	var diags model.DiagnosticErrorList

	symbol, ok := code.Symbol(target.SymbolID)

	if !ok {
		diags = append(diags, safetyDiagnostic(
			model.SeverityError, model.CodeUnresolvedSymbol,
			"selected symbol is absent from the code model",
		))

		return diags
	}

	if symbol.Generated || symbol.TestFile {
		diags = append(diags, safetyDiagnostic(
			model.SeverityWarning, model.CodeBroadPlan,
			"selection includes generated or test code",
		))
	}

	if target.ContextStrategy.Strategy == model.ContextStrategyRoot {
		diags = append(diags, safetyDiagnostic(
			model.SeverityWarning, model.CodeMissingContext,
			"explicit root span will not inherit caller context",
		))
	}

	if target.ErrorStrategy.Record && len(target.ErrorStrategy.Indexes) == 0 {
		diags = append(diags, safetyDiagnostic(
			model.SeverityWarning, model.CodeInvalidPolicy,
			"error recording requested but target has no error result",
		))
	}

	for _, attr := range target.Attributes {
		diags = append(diags, attributeSafety(code, *symbol, attr, secrets)...)
	}

	return diags
}

func attributeSafety(
	code *model.CodeModel, symbol model.Symbol, attr model.AttributePlan, secrets []string,
) model.DiagnosticErrorList {
	var diags model.DiagnosticErrorList

	_, err := AttributeAccessor(code, symbol, attr.From)
	if err != nil {
		diags = append(diags, safetyDiagnostic(model.SeverityError, model.CodeCaptureNotAllowed, err.Error()))
	}

	source := attr.Key + " " + attr.From.Argument + " " + attr.From.Result
	if matchesAny(source, secrets) || attr.Classification == model.ClassificationSecret {
		if !attr.Allow || attr.Classification != model.ClassificationSecret {
			diags = append(diags, safetyDiagnostic(
				model.SeverityError, model.CodeSecretAttribute,
				"sensitive attribute requires explicit secret classification and acknowledgment",
			))
		}
	}

	if piiNeedsAcknowledgment(source, attr) {
		diags = append(diags, safetyDiagnostic(model.SeverityWarning, model.CodePIIAttribute,
			"attribute may contain personal information; review classification and consent",
		))
	}

	if matchesAny(source, []string{"url", "query", "body", "uuid", "orderid", "userid", "timestamp", "message"}) {
		diags = append(diags, safetyDiagnostic(
			model.SeverityWarning, model.CodeCardinalityWarning,
			"attribute may have unbounded cardinality; prefer a bounded category",
		))
	}

	return diags
}

func matchesAny(value string, patterns []string) bool {
	normalize := func(text string) string {
		return strings.Map(func(r rune) rune {
			if r == '_' || r == '-' || r == '.' {
				return -1
			}

			return r
		}, strings.ToLower(text))
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
	tokens := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	return slices.Contains(tokens, "ip")
}

func safetyDiagnostic(severity model.Severity, code model.Code, message string) model.DiagnosticError {
	return model.DiagnosticError{
		Severity: severity,
		Code:     code,
		Message:  message,
		RuleID:   "",
		Symbol:   "",
		File:     "",
		Line:     0,
	}
}

func piiNeedsAcknowledgment(source string, attr model.AttributePlan) bool {
	classified := attr.Classification == model.ClassificationPII || attr.Classification == model.ClassificationSecret
	if attr.Allow && classified {
		return false
	}

	patterns := []string{"email", "phone", "address", "name", "userid", "customerid", "ipaddress", "deviceid"}

	return matchesAny(source, patterns) || hasIPToken(source) || attr.Classification == model.ClassificationPII
}
