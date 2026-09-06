package policy

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func validPolicyModel() *model.Policy {
	return &model.Policy{APIVersion: model.APIVersionV1Alpha1, Kind: model.KindInstrumentationPlan,
		Backend: model.BackendConfig{Name: "otelc", Version: "v0.1.0"},
		Rules:   []model.Rule{{ID: "operations", Match: model.Match{Functions: []string{"Run"}}}}}
}

func TestRejectUnsafeAndInvalidDefaults(t *testing.T) {
	for name, mutate := range map[string]func(*model.Policy){
		"argument capture": func(p *model.Policy) { p.Defaults.Attributes.Arguments = true },
		"result capture":   func(p *model.Policy) { p.Defaults.Attributes.Results = true },
		"unknown context":  func(p *model.Policy) { p.Defaults.Context.Mode = "ambient" },
		"default template": func(p *model.Policy) { p.Defaults.SpanName = "{{unknown}}" },
		"backend":          func(p *model.Policy) { p.Backend.Name = "unknown" },
		"ownership":        func(p *model.Policy) { p.Rules[0].Match.Ownership = "unknown" },
		"empty pattern":    func(p *model.Policy) { p.Rules[0].Match.Functions = []string{""} },
		"duplicate attribute": func(p *model.Policy) {
			p.Rules[0].Attributes = []model.AttributeRule{{Key: "kind", From: model.AttributeSource{Constant: "x"}}, {Key: "kind", From: model.AttributeSource{Constant: "y"}}}
		},
		"reserved key": func(p *model.Policy) {
			p.Rules[0].Attributes = []model.AttributeRule{{Key: "Otel.private", From: model.AttributeSource{Constant: "x"}}}
		},
		"duplicate exclusion": func(p *model.Policy) {
			p.Exclusions = []model.Exclusion{{ID: "x", Match: model.Match{Functions: []string{"A"}}}, {ID: "x", Match: model.Match{Functions: []string{"B"}}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := validPolicyModel()
			mutate(p)
			if !Validate(p).HasErrors() {
				t.Fatal("invalid policy accepted")
			}
		})
	}
}

func TestParseRejectsTrailingDocuments(t *testing.T) {
	if _, err := Parse([]byte("apiVersion: otelplan.io/v1alpha1\n---\nrules: []\n")); err == nil {
		t.Fatal("second YAML document ignored")
	}
}

func TestParseDefaultsPreserveExplicitFalse(t *testing.T) {
	p, err := Parse([]byte("apiVersion: otelplan.io/v1alpha1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !p.Defaults.Errors.Record || p.Defaults.Context.Mode != model.ContextModeRequire {
		t.Fatalf("unsafe or missing defaults: %+v", p.Defaults)
	}
	p, err = Parse([]byte("defaults:\n  errors:\n    record: false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Defaults.Errors.Record {
		t.Fatal("explicit false overwritten")
	}
}
