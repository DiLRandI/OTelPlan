package resolve_test

import (
	"reflect"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/resolve"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestResolveExclusionDecisionOrder(t *testing.T) {
	t.Parallel()

	policy, code := resolverInputs()
	local := policy.Rules[0]
	local.ID = "a"
	local.Exclude = &local.Match
	policy.Rules = append(policy.Rules, local)

	unmatched := local.Match
	unmatched.Functions = []string{"Other"}
	policy.Exclusions = []model.Exclusion{
		{ID: "z", Match: local.Match},
		{ID: "x", Match: unmatched},
	}

	result := resolve.Resolve(policy, code)
	if result.Diagnostics.HasErrors() || len(result.Plan.Targets) != 0 {
		t.Fatalf("excluded target should produce no errors or selected targets: %+v", result)
	}

	wantExplanations := []resolve.Explanation{{
		SymbolID: code.Symbols[0].ID, Selected: false,
		Decisions: []resolve.Decision{
			{RuleID: "a", Stage: "include", Matched: true, Reason: "selector matched"},
			{RuleID: "a", Stage: "local-exclusion", Matched: true, Reason: "selector matched"},
			{RuleID: "business", Stage: "include", Matched: true, Reason: "selector matched"},
			{RuleID: "x", Stage: "global-exclusion", Matched: false, Reason: "selector did not match"},
			{RuleID: "z", Stage: "global-exclusion", Matched: true, Reason: "selector matched"},
		},
	}}
	if !reflect.DeepEqual(result.Explanations, wantExplanations) {
		t.Fatalf("explanations = %+v; want %+v", result.Explanations, wantExplanations)
	}

	wantSkipped := []model.SkippedTarget{
		{SymbolID: code.Symbols[0].ID, RuleID: "a", Reason: model.SkipExcluded},
		{SymbolID: code.Symbols[0].ID, RuleID: "z", Reason: model.SkipExcluded},
	}
	if !reflect.DeepEqual(result.Plan.Skipped, wantSkipped) {
		t.Fatalf("skipped targets = %+v; want %+v", result.Plan.Skipped, wantSkipped)
	}
}
