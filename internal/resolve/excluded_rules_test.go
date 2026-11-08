package resolve_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/model"
	"github.com/DiLRandI/OTelPlan/internal/resolve"
)

func TestResolveWarnsForFullyExcludedRules(t *testing.T) {
	t.Parallel()

	for _, stage := range []string{"local", "global", "both"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()

			policy, code := resolverInputs()
			if stage != "global" {
				match := policy.Rules[0].Match
				policy.Rules[0].Exclude = &match
			}

			if stage != "local" {
				policy.Exclusions = []model.Exclusion{{ID: "excluded", Match: policy.Rules[0].Match}}
			}

			result := resolve.Resolve(policy, code)

			want := model.DiagnosticErrorList{{
				Severity: model.SeverityWarning, Code: "OTP1006", RuleID: "business",
				Message: "all matched symbols are excluded; rule selects no targets",
				Symbol:  "", File: "", Line: 0,
			}}
			if !reflect.DeepEqual(result.Diagnostics, want) || len(result.Plan.Targets) != 0 ||
				len(result.Explanations) != 1 || result.Explanations[0].Selected {
				t.Fatalf("fully %s-excluded rule: %+v", stage, result)
			}
		})
	}
}

func TestResolveDoesNotWarnForPartiallyExcludedRules(t *testing.T) {
	t.Parallel()

	policy, code := resolverInputs()
	other := code.Symbols[0]
	other.ID, other.Name = "example.com/app.Other", "Other"
	code.Symbols = append(code.Symbols, other)
	policy.Rules[0].Match.Functions = []string{"Run", "Other"}
	policy.Exclusions = []model.Exclusion{{ID: "excluded", Match: functionMatch([]string{"Run"})}}

	result := resolve.Resolve(policy, code)
	if len(result.Diagnostics) != 0 || len(result.Plan.Targets) != 1 || result.Plan.Targets[0].SymbolID != other.ID {
		t.Fatalf("partially excluded rule lost its effective match: %+v", result)
	}
}

func TestResolveCombinesLocalAndGlobalExclusionsAcrossSymbols(t *testing.T) {
	t.Parallel()

	policy, code := resolverInputs()
	other := code.Symbols[0]
	other.ID, other.Name = "example.com/app.Other", "Other"
	code.Symbols = append(code.Symbols, other)
	policy.Rules[0].Match.Functions = []string{"Run", "Other"}
	local := functionMatch([]string{"Run"})
	policy.Rules[0].Exclude = &local
	policy.Exclusions = []model.Exclusion{{ID: "excluded", Match: functionMatch([]string{"Other"})}}

	result := resolve.Resolve(policy, code)
	if len(result.Plan.Targets) != 0 || len(result.Plan.Skipped) != 2 || len(result.Diagnostics) != 1 ||
		result.Diagnostics[0].Code != "OTP1006" || result.Diagnostics[0].RuleID != "business" {
		t.Fatalf("combined exclusions lost the rule warning: %+v", result)
	}
}

func TestResolveReportsExcludedRuleAlongsideEffectiveRule(t *testing.T) {
	t.Parallel()

	policy, code := resolverInputs()
	other := code.Symbols[0]
	other.ID, other.Name = "example.com/app.Other", "Other"
	code.Symbols = append(code.Symbols, other)
	local := policy.Rules[0].Match
	policy.Rules[0].Exclude = &local

	var effective model.Rule

	effective.ID, effective.Match = "effective", functionMatch([]string{"Other"})
	policy.Rules = append(policy.Rules, effective)

	result := resolve.Resolve(policy, code)
	if len(result.Plan.Targets) != 1 || result.Plan.Targets[0].SymbolID != other.ID || len(result.Diagnostics) != 1 ||
		result.Diagnostics[0].Code != "OTP1006" || result.Diagnostics[0].RuleID != "business" {
		t.Fatalf("effective rule hid another rule's exclusions: %+v", result)
	}
}

func TestResolveDoesNotConfuseActiveRulesWithExclusions(t *testing.T) {
	t.Parallel()

	expected := map[string]model.Code{
		"conflict": model.CodeConflictingRules, "missing context": model.CodeMissingContext,
		"no matches": model.CodeUnresolvedSymbol,
	}

	for _, scenario := range []string{"identical overlap", "conflict", "missing context", "no matches"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()

			policy, code := resolverInputs()
			prepareActiveRuleScenario(policy, code, scenario)
			result := resolve.Resolve(policy, code)

			for _, diagnostic := range result.Diagnostics {
				if diagnostic.Code == "OTP1006" {
					t.Fatalf("active/unmatched %s rule reported as excluded: %+v", scenario, result)
				}
			}

			if scenario == "identical overlap" {
				if len(result.Diagnostics) != 0 || len(result.Plan.Targets) != 1 {
					t.Fatalf("identical overlap changed: %+v", result)
				}

				return
			}

			if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != expected[scenario] {
				t.Fatalf("%s lost its existing validation diagnostic: %+v", scenario, result)
			}
		})
	}
}

func prepareActiveRuleScenario(policy *model.Policy, code *model.CodeModel, scenario string) {
	switch scenario {
	case "identical overlap", "conflict":
		rule := policy.Rules[0]
		rule.ID = "other"

		if scenario == "conflict" {
			rule.Span = &model.SpanConfig{Name: "different", Kind: ""}
		}

		policy.Rules = append(policy.Rules, rule)
	case "missing context":
		code.Symbols[0].ContextIndexes = nil
	case "no matches":
		policy.Rules[0].Match.Functions = []string{"Missing"}
	}
}

func TestResolveExcludedRuleWarningsAreDeterministicAndImmutable(t *testing.T) {
	t.Parallel()

	policy, code := resolverInputs()
	duplicate := policy.Rules[0]
	duplicate.ID = "another"
	policy.Rules = append(policy.Rules, duplicate)
	policy.Exclusions = []model.Exclusion{
		{ID: "z", Match: duplicate.Match}, {ID: "a", Match: duplicate.Match},
	}

	before, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}

	first := resolve.Resolve(policy, code)

	after, err := json.Marshal(policy)
	if err != nil || string(before) != string(after) {
		t.Fatal("resolution changed the caller's policy", err)
	}

	slices.Reverse(policy.Rules)
	slices.Reverse(policy.Exclusions)

	second := resolve.Resolve(policy, code)
	if !reflect.DeepEqual(first, second) || len(first.Diagnostics) != 2 ||
		first.Diagnostics[0].RuleID != "another" || first.Diagnostics[1].RuleID != "business" {
		t.Fatalf("excluded-rule ordering changed: first=%+v second=%+v", first, second)
	}
}
