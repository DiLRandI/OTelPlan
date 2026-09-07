package otelc

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"gopkg.in/yaml.v3"
)

func ruleFixture() (*model.CodeModel, model.ResolvedPlan) {
	code := &model.CodeModel{Symbols: []model.Symbol{
		{ID: "example.com/app.Run", Kind: model.SymbolFunction, PackageImportPath: "example.com/app", PackageName: "app", Name: "Run", HasBody: true, Signature: "func()"},
		{ID: "example.com/app.(*Worker).Run", Kind: model.SymbolMethod, PackageImportPath: "example.com/app", PackageName: "app", Name: "Run", Receiver: &model.Receiver{Type: "Worker", Pointer: true}, HasBody: true, Signature: "func()"},
		{ID: "example.com/app.(Worker).Other", Kind: model.SymbolMethod, PackageImportPath: "example.com/app", PackageName: "app", Name: "Other", Receiver: &model.Receiver{Type: "Worker"}, HasBody: true, Signature: "func()"},
	}}
	plan := model.ResolvedPlan{}
	for _, symbol := range code.Symbols {
		plan.Targets = append(plan.Targets, model.ResolvedTarget{SymbolID: symbol.ID, Signature: symbol.Signature, RuleID: "chosen", ContextStrategy: model.ContextStrategy{Strategy: model.ContextStrategyRoot}})
	}
	return code, plan
}

func TestExactRuleRendering(t *testing.T) {
	code, plan := ruleFixture()
	original := append([]model.ResolvedTarget(nil), plan.Targets...)
	data, bindings, err := RenderRules(SupportedVersion, code, plan, "example.com/app/hooks")
	if err != nil {
		t.Fatal(err)
	}
	var rules map[string]functionRule
	if err := yaml.Unmarshal(data, &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) != 3 || len(bindings) != 3 {
		t.Fatalf("wrong target count: %s", data)
	}
	receivers := map[string]bool{}
	for _, rule := range rules {
		if rule.Target != "example.com/app" || len(rule.Actions) != 1 || rule.Actions[0].Hooks.Path != "example.com/app/hooks" {
			t.Fatalf("inexact rule: %+v", rule)
		}
		receivers[rule.Where.Receiver] = true
	}
	if !receivers[""] || !receivers["Worker"] || !receivers["*Worker"] {
		t.Fatalf("receiver identity lost: %s", data)
	}
	if !reflect.DeepEqual(plan.Targets, original) {
		t.Fatal("render mutated caller")
	}
	plan.Targets[0], plan.Targets[2] = plan.Targets[2], plan.Targets[0]
	reordered, rebound, err := RenderRules(SupportedVersion, code, plan, "example.com/app/hooks")
	if err != nil || !bytes.Equal(data, reordered) || !reflect.DeepEqual(bindings, rebound) {
		t.Fatal("input ordering changed generated artifacts")
	}
	if !strings.Contains(string(data), "policy rule") {
		t.Fatal("source policy mapping missing")
	}
}

func TestRuleRenderingRejectsUnrepresentableTargets(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*model.CodeModel, *model.ResolvedPlan)
		path   string
	}{
		{name: "duplicate", change: func(_ *model.CodeModel, p *model.ResolvedPlan) { p.Targets = append(p.Targets, p.Targets[0]) }},
		{name: "main mapping", change: func(c *model.CodeModel, _ *model.ResolvedPlan) { c.Symbols[0].PackageName = "main" }},
		{name: "identity mismatch", change: func(c *model.CodeModel, _ *model.ResolvedPlan) { c.Symbols[0].Name = "Other" }},
		{name: "receiver mismatch", change: func(c *model.CodeModel, _ *model.ResolvedPlan) { c.Symbols[1].Receiver.Pointer = false }},
		{name: "generic", change: func(c *model.CodeModel, _ *model.ResolvedPlan) {
			c.Symbols[0].Generics = &model.GenericInfo{TypeParams: []string{"T"}}
		}},
		{name: "self instrumentation", path: "example.com/app"},
		{name: "invalid hook path", path: "../hooks"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, plan := ruleFixture()
			if tc.change != nil {
				tc.change(code, &plan)
			}
			path := tc.path
			if path == "" {
				path = "example.com/app/hooks"
			}
			if data, bindings, err := RenderRules(SupportedVersion, code, plan, path); err == nil || data != nil || bindings != nil {
				t.Fatal("unsupported plan produced partial artifacts")
			}
		})
	}
}
