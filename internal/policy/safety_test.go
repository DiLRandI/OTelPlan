package policy_test

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/policy"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func validPolicyModel() *model.Policy {
	return &model.Policy{
		APIVersion: model.APIVersionV1Alpha1,
		Kind:       model.KindInstrumentationPlan,
		Project:    model.ProjectConfig{Packages: nil, IncludeTests: false, IncludeDependencies: false, BuildTags: nil},
		Backend:    model.BackendConfig{Name: "otelc", Version: "v0.1.0"},
		Defaults: model.Defaults{
			SpanName:   "",
			Context:    model.ContextDefaults{Mode: ""},
			Errors:     model.ErrorDefaults{Record: false},
			Attributes: model.CaptureDefaults{Arguments: false, Results: false},
		},
		Rules: []model.Rule{{
			ID: "operations", Description: "", Match: testFunctionMatch("Run"),
			Exclude: nil, Span: nil, Errors: nil, Attributes: nil,
		}},
		Exclusions: nil,
	}
}

func TestRejectUnsafeAndInvalidDefaults(t *testing.T) {
	t.Parallel()

	for name, mutate := range map[string]func(*model.Policy){
		"argument capture": func(p *model.Policy) { p.Defaults.Attributes.Arguments = true },
		"result capture":   func(p *model.Policy) { p.Defaults.Attributes.Results = true },
		"unknown context":  func(p *model.Policy) { p.Defaults.Context.Mode = "ambient" },
		"default template": func(p *model.Policy) { p.Defaults.SpanName = "{{unknown}}" },
		"backend":          func(p *model.Policy) { p.Backend.Name = "unknown" },
		"ownership":        func(p *model.Policy) { p.Rules[0].Match.Ownership = "unknown" },
		"empty pattern":    func(p *model.Policy) { p.Rules[0].Match.Functions = []string{""} },
		"duplicate attribute": func(p *model.Policy) {
			p.Rules[0].Attributes = []model.AttributeRule{testConstantAttribute("kind", "x"), testConstantAttribute("kind", "y")}
		},
		"reserved key": func(p *model.Policy) {
			p.Rules[0].Attributes = []model.AttributeRule{testConstantAttribute("Otel.private", "x")}
		},
		"duplicate exclusion": func(p *model.Policy) {
			p.Exclusions = []model.Exclusion{
				{ID: "x", Match: testFunctionMatch("A")},
				{ID: "x", Match: testFunctionMatch("B")},
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := validPolicyModel()
			mutate(p)

			if !policy.Validate(p).HasErrors() {
				t.Fatal("invalid policy accepted")
			}
		})
	}
}

func TestParseRejectsTrailingDocuments(t *testing.T) {
	t.Parallel()

	_, err := policy.Parse([]byte("apiVersion: otelplan.io/v1alpha1\n---\nrules: []\n"))
	if err == nil {
		t.Fatal("second YAML document ignored")
	}
}

func TestParseDefaultsPreserveExplicitFalse(t *testing.T) {
	t.Parallel()

	parsed, err := policy.Parse([]byte("apiVersion: otelplan.io/v1alpha1\n"))
	if err != nil {
		t.Fatal(err)
	}

	if !parsed.Defaults.Errors.Record || parsed.Defaults.Context.Mode != model.ContextModeRequire {
		t.Fatalf("unsafe or missing defaults: %+v", parsed.Defaults)
	}

	parsed, err = policy.Parse([]byte("defaults:\n  errors:\n    record: false\n"))
	if err != nil {
		t.Fatal(err)
	}

	if parsed.Defaults.Errors.Record {
		t.Fatal("explicit false overwritten")
	}
}

func testFunctionMatch(name string) model.Match {
	return model.Match{
		Packages: nil, Files: nil, Symbols: nil, Functions: []string{name},
		Receivers: nil, Methods: nil, Implements: nil,
		Exported: nil, HasContext: nil, ReturnsError: nil, Ownership: "",
	}
}

func testConstantAttribute(key, value string) model.AttributeRule {
	return model.AttributeRule{
		Key:    key,
		From:   model.AttributeSource{Argument: "", Result: "", Constant: value},
		Safety: nil,
	}
}
