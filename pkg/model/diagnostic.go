package model

import "fmt"

// Severity classifies a diagnostic as an error, warning, or informational
// message.
type Severity string

// Diagnostic severities used by validation and compilation reports.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Code is the stable identifier for a diagnostic category.
type Code string

// Diagnostic codes identify selector, context, safety, backend, and lockfile
// validation outcomes.
const (
	CodeInvalidSelector        Code = "OTP1001"
	CodeUnresolvedSymbol       Code = "OTP1002"
	CodeConflictingRules       Code = "OTP1003"
	CodeUnknownTemplateVar     Code = "OTP1004"
	CodeInvalidPolicy          Code = "OTP1005"
	CodeMissingContext         Code = "OTP3001"
	CodeMultipleContexts       Code = "OTP3002"
	CodeSecretAttribute        Code = "OTP4001"
	CodePIIAttribute           Code = "OTP4002"
	CodeCardinalityWarning     Code = "OTP4003"
	CodeBroadPlan              Code = "OTP4004"
	CodeCaptureNotAllowed      Code = "OTP4005"
	CodeBackendUnsupported     Code = "OTP5001"
	CodeBackendVersionMismatch Code = "OTP5002"
	CodeCompilationFailed      Code = "OTP5003"
	CodeArtifactOutput         Code = "OTP5004"
	CodeStaleLockfile          Code = "OTP6001"
)

// DiagnosticError describes one explainable problem or notice, with optional policy
// rule, symbol, and source location context.
type DiagnosticError struct {
	Severity Severity `json:"severity"`
	Code     Code     `json:"code"`
	Message  string   `json:"message"`
	RuleID   string   `json:"rule,omitempty"`
	Symbol   SymbolID `json:"symbol,omitempty"`
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
}

// Error formats the severity, diagnostic code, message, and optional rule ID.
func (d DiagnosticError) Error() string {
	if d.RuleID != "" {
		return fmt.Sprintf("%s %s: %s (rule %s)", d.Severity, d.Code, d.Message, d.RuleID)
	}

	return fmt.Sprintf("%s %s: %s", d.Severity, d.Code, d.Message)
}

// DiagnosticErrorList is an ordered collection of diagnostics produced by a model
// operation.
type DiagnosticErrorList []DiagnosticError

// Errors returns diagnostics with error severity, preserving their order.
func (l DiagnosticErrorList) Errors() DiagnosticErrorList {
	var out DiagnosticErrorList

	for _, d := range l {
		if d.Severity == SeverityError {
			out = append(out, d)
		}
	}

	return out
}

// Warnings returns diagnostics with warning severity, preserving their order.
// Callers may treat these as failures in strict mode.
func (l DiagnosticErrorList) Warnings() DiagnosticErrorList {
	var out DiagnosticErrorList

	for _, d := range l {
		if d.Severity == SeverityWarning {
			out = append(out, d)
		}
	}

	return out
}

// HasErrors reports whether the list contains at least one error diagnostic.
func (l DiagnosticErrorList) HasErrors() bool {
	return len(l.Errors()) > 0
}
