package policy_test

import (
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/policy"
)

func TestStringerPolicyOptInRequiresArgumentOrResult(t *testing.T) {
	t.Parallel()

	for _, from := range []string{"argument: req", "result: output", "constant: value", "symbol: name", "errorType: err"} {
		document := `apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
rules:
- id: typed
  match: {functions: [Run]}
  attributes:
  - key: display
    stringer: true
    from: {` + from + `}
`

		loaded, err := policy.Parse([]byte(document))
		if err != nil {
			t.Fatal(err)
		}

		diagnostics := policy.Validate(loaded)

		allowed := strings.HasPrefix(from, "argument:") || strings.HasPrefix(from, "result:")
		if diagnostics.HasErrors() == allowed {
			t.Fatalf("source=%s diagnostics=%+v", from, diagnostics)
		}

		if !loaded.Rules[0].Attributes[0].Stringer {
			t.Fatal("explicit flag lost during YAML decode")
		}
	}
}
