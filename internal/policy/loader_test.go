package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	p, err := Parse([]byte(validPolicy))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.APIVersion != model.APIVersionV1Alpha1 {
		t.Errorf("apiVersion = %q", p.APIVersion)
	}
	if p.Backend.Name != model.BackendNameOTelC {
		t.Errorf("backend = %q", p.Backend.Name)
	}
	if len(p.Rules) != 1 || p.Rules[0].ID != "checkout" {
		t.Errorf("rules = %+v", p.Rules)
	}
	if p.Rules[0].Match.Exported == nil || !*p.Rules[0].Match.Exported {
		t.Error("exported should be true")
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	data := validPolicy + "\nunknownField: true\n"
	if _, err := Parse([]byte(data)); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadExamplePolicies(t *testing.T) {
	for _, name := range []string{"otelplan.yaml", "otelplan.minimal.yaml"} {
		path := filepath.Join("..", "..", "examples", name)
		if _, err := os.Stat(path); err != nil {
			t.Skipf("examples not available on this branch: %v", err)
		}
		p, err := Load(path)
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		if diags := Validate(p); diags.HasErrors() {
			t.Errorf("%s: unexpected errors: %v", name, diags.Errors())
		}
	}
}

func TestValidateErrors(t *testing.T) {
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
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Parse([]byte(tc.yaml))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			diags := Validate(p)
			if !diags.HasErrors() {
				t.Fatalf("expected errors, got %+v", diags)
			}
			found := false
			for _, d := range diags.Errors() {
				if d.Code == tc.want {
					found = true
				}
			}
			if !found {
				t.Errorf("want code %s, got %+v", tc.want, diags.Errors())
			}
		})
	}
}

func TestValidateRejectsReservedAttributeKey(t *testing.T) {
	data := strings.Replace(validPolicy, "      exported: true",
		"      exported: true\n    attributes:\n      - key: \"otel.scope.name\"\n        from:\n          constant: \"x\"", 1)
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	diags := Validate(p)
	if !diags.HasErrors() {
		t.Fatal("expected error for reserved otel. key")
	}
}

func TestValidateAcceptsAcknowledgedPII(t *testing.T) {
	data := strings.Replace(validPolicy, "      exported: true",
		"      exported: true\n    attributes:\n      - key: \"customer.email\"\n        from:\n          argument: \"customer.Email\"\n        safety:\n          classification: pii\n          allow: true", 1)
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if diags := Validate(p); diags.HasErrors() {
		t.Errorf("unexpected errors: %+v", diags.Errors())
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	p, err := Parse([]byte(validPolicy))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	path := filepath.Join(t.TempDir(), "otelplan.yaml")
	if err := os.WriteFile(path, []byte(validPolicy), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Backend.Version != p.Backend.Version {
		t.Errorf("version = %q, want %q", got.Backend.Version, p.Backend.Version)
	}
}
