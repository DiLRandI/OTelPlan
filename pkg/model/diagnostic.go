package model

import "fmt"

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

type Code string

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

type Diagnostic struct {
	Severity Severity `json:"severity"`
	Code     Code     `json:"code"`
	Message  string   `json:"message"`
	RuleID   string   `json:"rule,omitempty"`
	Symbol   SymbolID `json:"symbol,omitempty"`
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
}

func (d Diagnostic) Error() string {
	if d.RuleID != "" {
		return fmt.Sprintf("%s %s: %s (rule %s)", d.Severity, d.Code, d.Message, d.RuleID)
	}
	return fmt.Sprintf("%s %s: %s", d.Severity, d.Code, d.Message)
}

type DiagnosticList []Diagnostic

func (l DiagnosticList) Errors() DiagnosticList {
	var out DiagnosticList
	for _, d := range l {
		if d.Severity == SeverityError {
			out = append(out, d)
		}
	}
	return out
}

func (l DiagnosticList) Warnings() DiagnosticList {
	var out DiagnosticList
	for _, d := range l {
		if d.Severity == SeverityWarning {
			out = append(out, d)
		}
	}
	return out
}

func (l DiagnosticList) HasErrors() bool {
	return len(l.Errors()) > 0
}
