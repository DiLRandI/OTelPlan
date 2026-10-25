package policy_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/policy"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestErrorTypeAttributeSourceParseValidateAndRoundTrip(t *testing.T) {
	t.Parallel()

	document := `apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
rules:
- id: classify
  match: {functions: [Run]}
  attributes:
  - key: error.type
    from: {errorType: "0"}
`

	parsed, err := policy.Parse([]byte(document))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if diagnostics := policy.Validate(parsed); diagnostics.HasErrors() {
		t.Fatalf("validate: %+v", diagnostics.Errors())
	}

	got := parsed.Rules[0].Attributes[0].From

	var want model.AttributeSource

	want.ErrorType = "0"
	if got != want {
		t.Fatalf("source = %+v, want error type result index", got)
	}

	encoded, err := json.Marshal(parsed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if !strings.Contains(string(encoded), `"errorType":"0"`) {
		t.Fatalf("round trip omitted errorType: %s", encoded)
	}

	if strings.Contains(string(encoded), `"argument"`) || strings.Contains(string(encoded), `"result"`) {
		t.Fatalf("round trip added another source: %s", encoded)
	}
}

func TestErrorTypeAttributeSourceRejectsEmptyAndMixedSources(t *testing.T) {
	t.Parallel()

	for name, source := range map[string]string{
		"empty":  `errorType: ""`,
		"mixed":  `errorType: caller-private-value, result: result`,
		"symbol": `errorType: err, symbol: name`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			document := errorTypePolicy(source)

			parsed, err := policy.Parse([]byte(document))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			diagnostics := policy.Validate(parsed)
			if !diagnostics.HasErrors() {
				t.Fatalf("source accepted: %+v", parsed.Rules[0].Attributes[0].From)
			}

			if strings.Contains(diagnostics[0].Message, "caller-private-value") {
				t.Fatalf("diagnostic echoed source value: %+v", diagnostics)
			}
		})
	}
}

func TestErrorTypeAttributeSourceRejectsMalformedYAMLKind(t *testing.T) {
	t.Parallel()

	_, err := policy.Parse([]byte(errorTypePolicy("errorType: [0]")))
	if err == nil {
		t.Fatal("collection errorType accepted; expected a selector string")
	}
}

func TestPolicyDefaultsDoNotCaptureErrorTypes(t *testing.T) {
	t.Parallel()

	parsed, err := policy.Parse([]byte(validPolicy))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if parsed.Defaults.Attributes.Arguments || parsed.Defaults.Attributes.Results {
		t.Fatalf("capture defaults enabled: %+v", parsed.Defaults.Attributes)
	}

	for _, rule := range parsed.Rules {
		for _, attribute := range rule.Attributes {
			if attribute.From.ErrorType != "" {
				t.Fatalf("default policy unexpectedly captured error type: %+v", attribute)
			}
		}
	}
}

func errorTypePolicy(source string) string {
	return `apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
rules:
- id: classify
  match: {functions: [Run]}
  attributes:
  - key: error.type
    from: {` + source + `}
`
}
