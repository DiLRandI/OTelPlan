package policy_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/policy"
)

func TestValidateSymbolAttributeSources(t *testing.T) {
	t.Parallel()
	assertValidSymbolSources(t)
	assertInvalidSymbolSources(t)
}

func assertValidSymbolSources(t *testing.T) {
	t.Helper()

	for _, field := range []string{"id", "package", "packageName", "name", "receiver", "signature", "kind"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			document := symbolAttributePolicy("symbol: " + field)

			parsed, err := policy.Parse([]byte(document))
			if err != nil {
				t.Fatal(err)
			}

			before, err := json.Marshal(parsed)
			if err != nil {
				t.Fatal(err)
			}

			if diagnostics := policy.Validate(parsed); diagnostics.HasErrors() {
				t.Fatalf("symbol field %s rejected: %+v", field, diagnostics)
			}

			after, err := json.Marshal(parsed)
			if err != nil || string(before) != string(after) {
				t.Fatalf("policy validation mutated caller policy: %v", err)
			}
		})
	}
}

func assertInvalidSymbolSources(t *testing.T) {
	t.Helper()

	sources := []string{"symbol: ''", "symbol: private-marker", "symbol: name, constant: keep",
		"symbol: name, argument: arg"}
	for _, source := range sources {
		t.Run("invalid", func(t *testing.T) {
			t.Parallel()

			document := symbolAttributePolicy(source)

			parsed, err := policy.Parse([]byte(document))
			if err != nil {
				t.Fatal(err)
			}

			diagnostics := policy.Validate(parsed)

			if !diagnostics.HasErrors() || strings.Contains(diagnostics[0].Message, "private-marker") {
				t.Fatalf("invalid symbol source diagnostics=%+v", diagnostics)
			}
		})
	}
}

func symbolAttributePolicy(source string) string {
	if source == "" {
		source = "symbol: ''"
	}

	return `apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
rules:
- id: metadata
  match: {functions: [Run]}
  attributes:
  - key: metadata
    from: {` + source + `}
`
}
