package policy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/policy"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const validPolicy = `
apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend:
  name: otelc
  version: "v0.1.0"
rules:
  - id: checkout
    match:
      packages:
        - "github.com/acme/shop/internal/checkout"
      exported: true
`

func TestParseValidPolicy(t *testing.T) {
	t.Parallel()

	parsed, err := policy.Parse([]byte(validPolicy))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if parsed.APIVersion != model.APIVersionV1Alpha1 {
		t.Errorf("apiVersion = %q", parsed.APIVersion)
	}

	if parsed.Backend.Name != model.BackendNameOTelC {
		t.Errorf("backend = %q", parsed.Backend.Name)
	}

	if len(parsed.Rules) != 1 || parsed.Rules[0].ID != "checkout" {
		t.Errorf("rules = %+v", parsed.Rules)
	}

	if parsed.Rules[0].Match.Exported == nil || !*parsed.Rules[0].Match.Exported {
		t.Error("exported should be true")
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	data := validPolicy + "\nunknownField: true\n"

	_, err := policy.Parse([]byte(data))
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Parallel()

	_, err := policy.Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadExamplePolicies(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"otelplan.yaml", "otelplan.minimal.yaml"} {
		path := filepath.Join("..", "..", "examples", name)

		_, err := os.Stat(path)
		if err != nil {
			t.Skipf("examples not available on this branch: %v", err)
		}

		parsed, err := policy.Load(path)
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}

		if diags := policy.Validate(parsed); diags.HasErrors() {
			t.Errorf("%s: unexpected errors: %v", name, diags.Errors())
		}
	}
}

func TestValidateErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		yaml string
		want model.Code
	}{
		{
			name: "wrong apiVersion",
			yaml: strings.Replace(validPolicy, "otelplan.io/v1alpha1", "otelplan.io/v1", 1),
			want: model.CodeInvalidPolicy,
		},
		{
			name: "unpinned backend",
			yaml: strings.Replace(validPolicy, `"v0.1.0"`, `""`, 1),
			want: model.CodeBackendVersionMismatch,
		},
		{
			name: "duplicate rule ids",
			yaml: validPolicy + `
  - id: checkout
    match:
      methods: ["Create"]
`,
			want: model.CodeConflictingRules,
		},
		{
			name: "unknown template variable",
			yaml: strings.Replace(validPolicy, "exported: true", "exported: true\n    span:\n      name: \"{{bogus}}\"", 1),
			want: model.CodeUnknownTemplateVar,
		},
		{
			name: "empty match",
			yaml: `
apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend:
  name: otelc
  version: "v0.1.0"
rules:
  - id: empty
    match: {}
`,
			want: model.CodeInvalidSelector,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			checkPolicyDiagnostic(t, testCase.yaml, testCase.want)
		})
	}
}

func TestValidateRejectsReservedAttributeKey(t *testing.T) {
	t.Parallel()

	data := strings.Replace(validPolicy, "      exported: true",
		"      exported: true\n    attributes:\n"+
			"      - key: \"otel.scope.name\"\n        from:\n          constant: \"x\"", 1)

	parsed, err := policy.Parse([]byte(data))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	diags := policy.Validate(parsed)
	if !diags.HasErrors() {
		t.Fatal("expected error for reserved otel. key")
	}
}

func TestValidateAcceptsAcknowledgedPII(t *testing.T) {
	t.Parallel()

	data := strings.Replace(validPolicy, "      exported: true",
		"      exported: true\n    attributes:\n"+
			"      - key: \"customer.email\"\n        from:\n          argument: \"customer.Email\"\n"+
			"        safety:\n          classification: pii\n          allow: true", 1)

	parsed, err := policy.Parse([]byte(data))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if diags := policy.Validate(parsed); diags.HasErrors() {
		t.Errorf("unexpected errors: %+v", diags.Errors())
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	t.Parallel()

	parsed, err := policy.Parse([]byte(validPolicy))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	path := filepath.Join(t.TempDir(), "otelplan.yaml")

	err = os.WriteFile(path, []byte(validPolicy), 0o600)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := policy.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if got.Backend.Version != parsed.Backend.Version {
		t.Errorf("version = %q, want %q", got.Backend.Version, parsed.Backend.Version)
	}
}

func checkPolicyDiagnostic(t *testing.T, source string, want model.Code) {
	t.Helper()

	parsed, err := policy.Parse([]byte(source))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	diags := policy.Validate(parsed)
	if !diags.HasErrors() {
		t.Fatalf("expected errors, got %+v", diags)
	}

	found := false

	for _, d := range diags.Errors() {
		if d.Code == want {
			found = true
		}
	}

	if !found {
		t.Errorf("want code %s, got %+v", want, diags.Errors())
	}
}
