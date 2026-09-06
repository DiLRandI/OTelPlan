package lockfile

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func policyDigestFixture() *model.Policy {
	return &model.Policy{
		Backend: model.BackendConfig{Name: "otelc", Version: "v1.1.0"},
		Project: model.ProjectConfig{Packages: []string{"./b/...", "./a/..."}, BuildTags: []string{"two", "one"}},
		Rules: []model.Rule{
			{ID: "b", Match: model.Match{Functions: []string{"Second", "First"}}, Exclude: &model.Match{Files: []string{"b.go", "a.go"}}, Attributes: []model.AttributeRule{{Key: "b", From: model.AttributeSource{Constant: "b"}}, {Key: "a", From: model.AttributeSource{Constant: "a"}}}},
			{ID: "a", Match: model.Match{Packages: []string{"example.com/b", "example.com/a"}}},
		},
		Exclusions: []model.Exclusion{{ID: "second", Match: model.Match{Functions: []string{"B", "A"}}}, {ID: "first", Match: model.Match{Methods: []string{"Skip"}}}},
	}
}

func digestPolicy(t *testing.T, p *model.Policy) string {
	t.Helper()
	lock, err := Create(p, &model.CodeModel{GoVersion: "go1.27.0"}, model.ResolvedPlan{}, model.LockBackend{Name: p.Backend.Name, Version: p.Backend.Version}, Digest(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	return lock.PolicyDigest
}

func TestPolicyDigestIgnoresSetOrdering(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*model.Policy)
	}{
		{"rules", func(p *model.Policy) { slices.Reverse(p.Rules) }},
		{"exclusions", func(p *model.Policy) { slices.Reverse(p.Exclusions) }},
		{"selectors", func(p *model.Policy) {
			slices.Reverse(p.Rules[0].Match.Functions)
			slices.Reverse(p.Rules[1].Match.Packages)
			slices.Reverse(p.Rules[0].Exclude.Files)
			slices.Reverse(p.Exclusions[0].Match.Functions)
		}},
		{"tags", func(p *model.Policy) { slices.Reverse(p.Project.BuildTags) }},
		{"packages", func(p *model.Policy) { slices.Reverse(p.Project.Packages) }},
		{"attributes", func(p *model.Policy) { slices.Reverse(p.Rules[0].Attributes) }},
		{"duplicate selectors", func(p *model.Policy) { p.Rules[0].Match.Functions = append(p.Rules[0].Match.Functions, "First") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := policyDigestFixture()
			before := digestPolicy(t, p)
			tc.change(p)
			if digestPolicy(t, p) != before {
				t.Fatal("semantically irrelevant ordering changed policy digest")
			}
		})
	}
}

func TestPolicyDigestPreservesSemanticsAndCaller(t *testing.T) {
	p := policyDigestFixture()
	original, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	before := digestPolicy(t, p)
	after, err := json.Marshal(p)
	if err != nil || string(original) != string(after) {
		t.Fatal("canonicalization mutated caller")
	}
	if digestPolicy(t, p) != before {
		t.Fatal("policy digest is nondeterministic")
	}
	for _, change := range []func(*model.Policy){
		func(p *model.Policy) { p.Rules[0].Match.Functions[0] = "Changed" },
		func(p *model.Policy) { p.Rules[0].Attributes[0].From.Constant = "changed" },
		func(p *model.Policy) { p.Defaults.Errors.Record = true },
		func(p *model.Policy) { p.Rules[0].Span = &model.SpanConfig{Name: "changed"} },
	} {
		p := policyDigestFixture()
		change(p)
		if digestPolicy(t, p) == before {
			t.Fatal("semantic change ignored")
		}
	}
}
