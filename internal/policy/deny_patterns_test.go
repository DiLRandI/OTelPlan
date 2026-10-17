package policy_test

import (
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/policy"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestDenyPatternsAreOptionalAndValidated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		declaration string
		valid       bool
	}{
		{name: "omitted", declaration: "", valid: true},
		{name: "empty list", declaration: "denyPatterns: []", valid: true},
		{name: "valid fragment", declaration: "denyPatterns: [BANK_REFERENCE]", valid: true},
		{name: "empty pattern", declaration: `denyPatterns: [""]`, valid: false},
		{name: "whitespace only", declaration: `denyPatterns: ["   "]`, valid: false},
		{name: "separators only", declaration: `denyPatterns: ["._-"]`, valid: false},
		{name: "space", declaration: `denyPatterns: ["bank reference"]`, valid: false},
		{name: "tab", declaration: `denyPatterns: ["bank\treference"]`, valid: false},
		{name: "newline", declaration: `denyPatterns: ["bank\nreference"]`, valid: false},
		{name: "non-breaking space", declaration: `denyPatterns: ["bank\u00a0reference"]`, valid: false},
		{name: "em space", declaration: `denyPatterns: ["bank\u2003reference"]`, valid: false},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			checkDenyPatternValidity(t, testCase.declaration, testCase.valid)
		})
	}
}

func checkDenyPatternValidity(t *testing.T, declaration string, valid bool) {
	t.Helper()

	parsed, err := policy.Parse([]byte(validPolicy + "\n" + declaration + "\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	diagnostics := policy.Validate(parsed)
	if diagnostics.HasErrors() == valid {
		t.Fatalf("valid=%t diagnostics=%+v", valid, diagnostics)
	}

	for _, diagnostic := range diagnostics.Errors() {
		if diagnostic.Code != model.CodeInvalidPolicy || strings.Contains(diagnostic.Message, "bank reference") {
			t.Fatalf("unexpected invalid-pattern diagnostic: %+v", diagnostic)
		}
	}
}
