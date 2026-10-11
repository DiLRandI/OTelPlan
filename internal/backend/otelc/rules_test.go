package otelc_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
	"gopkg.in/yaml.v3"
)

func TestExactRuleRendering(t *testing.T) {
	t.Parallel()

	code, plan := bundleFixture()
	original := append([]model.ResolvedTarget(nil), plan.Targets...)

	data, bindings, err := otelc.RenderRules(otelc.SupportedVersion, code, plan, "example.com/app/hooks")
	if err != nil {
		t.Fatal(err)
	}

	var rules map[string]decodedFunctionRule

	err = yaml.Unmarshal(data, &rules)
	if err != nil {
		t.Fatal(err)
	}

	assertExactRenderedRules(t, data, rules, bindings)

	if !reflect.DeepEqual(plan.Targets, original) {
		t.Fatal("render mutated caller")
	}

	assertReorderedRenderingIsStable(t, code, plan, data, bindings)

	if !strings.Contains(string(data), "policy rule") {
		t.Fatal("source policy mapping missing")
	}
}

func assertExactRenderedRules(t *testing.T, data []byte,
	rules map[string]decodedFunctionRule, bindings []otelc.HookBinding) {
	t.Helper()

	if len(rules) != 3 || len(bindings) != 3 {
		t.Fatalf("wrong target count: %s", data)
	}

	receivers := map[string]bool{}

	for _, rule := range rules {
		if rule.Target != "example.com/app" || len(rule.Actions) != 1 ||
			rule.Actions[0].Hooks.Path != "example.com/app/hooks" {
			t.Fatalf("inexact rule: target=%q actions=%+v", rule.Target, rule.Actions)
		}

		receivers[rule.Where.Receiver] = true
	}

	if !receivers[""] || !receivers["Worker"] || !receivers["*Worker"] {
		t.Fatalf("receiver identity lost: %s", data)
	}
}

func assertReorderedRenderingIsStable(
	t *testing.T,
	code *model.CodeModel,
	plan model.ResolvedPlan,
	data []byte,
	bindings []otelc.HookBinding,
) {
	t.Helper()

	plan.Targets[0], plan.Targets[2] = plan.Targets[2], plan.Targets[0]

	reordered, rebound, err := otelc.RenderRules(otelc.SupportedVersion, code, plan, "example.com/app/hooks")
	if err != nil || !bytes.Equal(data, reordered) ||
		!reflect.DeepEqual(bindings, rebound) {
		t.Fatal("input ordering changed generated artifacts")
	}
}

type ruleMutationCase struct {
	name   string
	change func(*model.CodeModel, *model.ResolvedPlan)
	path   string
}

func TestRuleRenderingRejectsUnrepresentableTargets(t *testing.T) {
	t.Parallel()

	for _, testCase := range unrepresentableRuleCases() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			code, plan := bundleFixture()
			if testCase.change != nil {
				testCase.change(code, &plan)
			}

			path := testCase.path
			if path == "" {
				path = "example.com/app/hooks"
			}

			data, bindings, err := otelc.RenderRules(otelc.SupportedVersion, code, plan, path)
			if err == nil || data != nil || bindings != nil {
				t.Fatal("unsupported plan produced partial artifacts")
			}
		})
	}
}

func unrepresentableRuleCases() []ruleMutationCase {
	return []ruleMutationCase{
		{
			name: "duplicate",
			change: func(_ *model.CodeModel, plan *model.ResolvedPlan) {
				plan.Targets = append(plan.Targets, plan.Targets[0])
			},
			path: "",
		},
		{
			name: "main mapping",
			change: func(code *model.CodeModel, _ *model.ResolvedPlan) {
				code.Symbols[0].PackageName = "main"
			},
			path: "",
		},
		{
			name: "identity mismatch",
			change: func(code *model.CodeModel, _ *model.ResolvedPlan) {
				code.Symbols[0].Name = "Other"
			},
			path: "",
		},
		{
			name: "receiver mismatch",
			change: func(code *model.CodeModel, _ *model.ResolvedPlan) {
				code.Symbols[1].Receiver.Pointer = false
			},
			path: "",
		},
		{
			name: "generic argument context",
			change: func(code *model.CodeModel, plan *model.ResolvedPlan) {
				genericInfo := new(model.GenericInfo)
				genericInfo.TypeParams = []string{"T"}
				code.Symbols[0].Generics = genericInfo
				parameter := new(model.Parameter)
				parameter.Name = "ctx"
				parameter.Type = "context.Context"
				code.Symbols[0].Parameters = append(code.Symbols[0].Parameters, *parameter)
				code.Symbols[0].ContextIndexes = []int{0}
				code.Symbols[0].Signature = "func[T any](context.Context)"
				plan.Targets[0].Signature = code.Symbols[0].Signature
				plan.Targets[0].ContextStrategy.Strategy = model.ContextStrategyArgument
			},
			path: "",
		},
		{name: "self instrumentation", change: nil, path: "example.com/app"},
		{name: "invalid hook path", change: nil, path: "../hooks"},
	}
}

func TestRuleRenderingPreservesValidationErrorIdentity(t *testing.T) {
	t.Parallel()

	for _, testCase := range repeatedRuleErrorCases() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			code, plan := bundleFixture()
			if testCase.change != nil {
				testCase.change(code, &plan)
			}

			path := testCase.path
			if path == "" {
				path = "example.com/app/hooks"
			}

			assertRepeatedRuleError(t, code, plan, path, testCase.wantText)
		})
	}
}

type repeatedRuleErrorCase struct {
	name     string
	change   func(*model.CodeModel, *model.ResolvedPlan)
	path     string
	wantText string
}

func repeatedRuleErrorCases() []repeatedRuleErrorCase {
	return []repeatedRuleErrorCase{
		{
			name:     "invalid hook path",
			change:   nil,
			path:     "../hooks",
			wantText: "invalid generated hook import path",
		},
		{
			name: "duplicate target",
			change: func(_ *model.CodeModel, plan *model.ResolvedPlan) {
				plan.Targets = append(plan.Targets, plan.Targets[0])
			},
			path:     "",
			wantText: "duplicate backend target or generated rule identity",
		},
		{
			name: "canonical identity mismatch",
			change: func(code *model.CodeModel, _ *model.ResolvedPlan) {
				code.Symbols[0].Name = "Other"
			},
			path:     "",
			wantText: "backend target has inconsistent canonical identity",
		},
		{
			name: "receiver mismatch",
			change: func(code *model.CodeModel, _ *model.ResolvedPlan) {
				code.Symbols[1].Receiver.Pointer = false
			},
			path:     "",
			wantText: "backend target has inconsistent receiver identity",
		},
		{
			name:     "self instrumentation",
			change:   nil,
			path:     "example.com/app",
			wantText: "generated hooks cannot instrument their own package",
		},
	}
}

func assertRepeatedRuleError(t *testing.T, code *model.CodeModel, plan model.ResolvedPlan, path, wantText string) {
	t.Helper()

	firstData, firstBindings, firstErr := otelc.RenderRules(otelc.SupportedVersion, code, plan, path)
	if firstErr == nil || firstData != nil || firstBindings != nil {
		t.Fatalf("first call returned partial artifacts or no error: data=%q bindings=%v err=%v",
			firstData, firstBindings, firstErr)
	}

	if firstErr.Error() != wantText {
		t.Fatalf("first call error = %q, want %q", firstErr, wantText)
	}

	secondData, secondBindings, secondErr := otelc.RenderRules(otelc.SupportedVersion, code, plan, path)
	if secondErr == nil || secondData != nil || secondBindings != nil {
		t.Fatalf("second call returned partial artifacts or no error: data=%q bindings=%v err=%v",
			secondData, secondBindings, secondErr)
	}

	if !errors.Is(secondErr, firstErr) {
		t.Fatalf("repeated error does not preserve identity: first=%p %v, second=%p %v",
			firstErr, firstErr, secondErr, secondErr)
	}
}

type decodedFunctionRule struct {
	Target string `yaml:"target"`
	Where  struct {
		Function string `yaml:"func"`
		Receiver string `yaml:"recv,omitempty"`
	} `yaml:"where"`
	Actions []struct {
		Hooks struct {
			Before string `yaml:"before"`
			After  string `yaml:"after"`
			Path   string `yaml:"path"`
		} `yaml:"inject_hooks"`
	} `yaml:"do"`
}
