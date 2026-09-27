package lockfile_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func policyDigestFixture() *model.Policy {
	var defaults model.Defaults

	return &model.Policy{
		APIVersion: "", Kind: "", Defaults: defaults,
		Backend: model.BackendConfig{Name: "otelc", Version: "v1.1.0"},
		Project: model.ProjectConfig{
			Packages: []string{"./b/...", "./a/..."}, BuildTags: []string{"two", "one"},
			IncludeTests: false, IncludeDependencies: false,
		},
		Rules: []model.Rule{
			{
				ID: "b", Description: "", Span: nil, Errors: nil,
				Match: model.Match{
					Functions: []string{"Second", "First"}, Packages: nil, Files: nil, Symbols: nil,
					Receivers: nil, Methods: nil, Implements: nil, Exported: nil,
					HasContext: nil, ReturnsError: nil, Ownership: "",
				},
				Exclude: &model.Match{
					Files: []string{"b.go", "a.go"}, Packages: nil, Symbols: nil, Functions: nil,
					Receivers: nil, Methods: nil, Implements: nil, Exported: nil,
					HasContext: nil, ReturnsError: nil, Ownership: "",
				},
				Attributes: []model.AttributeRule{
					{Key: "b", From: model.AttributeSource{Constant: "b", Argument: "", Result: ""}, Safety: nil},
					{Key: "a", From: model.AttributeSource{Constant: "a", Argument: "", Result: ""}, Safety: nil},
				},
			},
			{
				ID: "a", Description: "", Exclude: nil, Span: nil, Errors: nil, Attributes: nil,
				Match: model.Match{
					Packages: []string{"example.com/b", "example.com/a"}, Files: nil, Symbols: nil, Functions: nil,
					Receivers: nil, Methods: nil, Implements: nil, Exported: nil,
					HasContext: nil, ReturnsError: nil, Ownership: "",
				},
			},
		},
		Exclusions: []model.Exclusion{
			{ID: "second", Match: model.Match{
				Functions: []string{"B", "A"}, Packages: nil, Files: nil, Symbols: nil,
				Receivers: nil, Methods: nil, Implements: nil, Exported: nil,
				HasContext: nil, ReturnsError: nil, Ownership: "",
			}},
			{ID: "first", Match: model.Match{
				Methods: []string{"Skip"}, Packages: nil, Files: nil, Symbols: nil, Functions: nil,
				Receivers: nil, Implements: nil, Exported: nil, HasContext: nil, ReturnsError: nil, Ownership: "",
			}},
		},
	}
}

func digestPolicy(t *testing.T, policy *model.Policy) string {
	t.Helper()

	var code model.CodeModel

	code.GoVersion = "go1.27.0"

	var plan model.ResolvedPlan

	var capabilities model.BackendCapabilities

	backend := model.LockBackend{
		Name: policy.Backend.Name, Version: policy.Backend.Version, Digest: "", Capabilities: capabilities,
	}

	lock, err := lockfile.Create(policy, &code, plan, backend, lockfile.Digest(nil), nil)
	if err != nil {
		t.Fatal(err)
	}

	return lock.PolicyDigest
}

func TestPolicyDigestIgnoresSetOrdering(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		change func(*model.Policy)
	}{
		{"rules", func(policy *model.Policy) { slices.Reverse(policy.Rules) }},
		{"exclusions", func(policy *model.Policy) { slices.Reverse(policy.Exclusions) }},
		{"selectors", func(policy *model.Policy) {
			slices.Reverse(policy.Rules[0].Match.Functions)
			slices.Reverse(policy.Rules[1].Match.Packages)
			slices.Reverse(policy.Rules[0].Exclude.Files)
			slices.Reverse(policy.Exclusions[0].Match.Functions)
		}},
		{"tags", func(policy *model.Policy) { slices.Reverse(policy.Project.BuildTags) }},
		{"packages", func(policy *model.Policy) { slices.Reverse(policy.Project.Packages) }},
		{"attributes", func(policy *model.Policy) { slices.Reverse(policy.Rules[0].Attributes) }},
		{"duplicate selectors", func(policy *model.Policy) {
			policy.Rules[0].Match.Functions = append(policy.Rules[0].Match.Functions, "First")
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			policy := policyDigestFixture()
			before := digestPolicy(t, policy)
			testCase.change(policy)

			if digestPolicy(t, policy) != before {
				t.Fatal("semantically irrelevant ordering changed policy digest")
			}
		})
	}
}

func TestPolicyDigestPreservesSemanticsAndCaller(t *testing.T) {
	t.Parallel()

	policy := policyDigestFixture()

	original, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}

	before := digestPolicy(t, policy)

	after, err := json.Marshal(policy)
	if err != nil || string(original) != string(after) {
		t.Fatal("canonicalization mutated caller")
	}

	if digestPolicy(t, policy) != before {
		t.Fatal("policy digest is nondeterministic")
	}

	for _, change := range []func(*model.Policy){
		func(policy *model.Policy) { policy.Rules[0].Match.Functions[0] = "Changed" },
		func(policy *model.Policy) { policy.Rules[0].Attributes[0].From.Constant = "changed" },
		func(policy *model.Policy) { policy.Defaults.Errors.Record = true },
		func(policy *model.Policy) { policy.Rules[0].Span = &model.SpanConfig{Name: "changed", Kind: ""} },
	} {
		policy := policyDigestFixture()
		change(policy)

		if digestPolicy(t, policy) == before {
			t.Fatal("semantic change ignored")
		}
	}
}
