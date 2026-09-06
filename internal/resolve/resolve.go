package resolve

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/DiLRandI/OTelPlan/internal/policy"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

type Decision struct {
	RuleID  string `json:"rule"`
	Stage   string `json:"stage"`
	Matched bool   `json:"matched"`
	Reason  string `json:"reason"`
}

type Explanation struct {
	SymbolID  model.SymbolID `json:"symbol"`
	Decisions []Decision     `json:"decisions"`
	Selected  bool           `json:"selected"`
}

type Result struct {
	Plan         model.ResolvedPlan   `json:"plan"`
	Diagnostics  model.DiagnosticList `json:"diagnostics"`
	Explanations []Explanation        `json:"explanations"`
}

func Resolve(p *model.Policy, code *model.CodeModel) Result {
	result := Result{Plan: model.ResolvedPlan{APIVersion: model.APIVersionV1Alpha1, Targets: []model.ResolvedTarget{}}, Diagnostics: policy.Validate(p)}
	if result.Diagnostics.HasErrors() {
		return result
	}
	if code == nil {
		result.Diagnostics = append(result.Diagnostics, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeInvalidPolicy, Message: "code model is required"})
		return result
	}
	rules := append([]model.Rule(nil), p.Rules...)
	exclusions := append([]model.Exclusion(nil), p.Exclusions...)
	symbols := append([]model.Symbol(nil), code.Symbols...)
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	sort.Slice(exclusions, func(i, j int) bool { return exclusions[i].ID < exclusions[j].ID })
	sort.Slice(symbols, func(i, j int) bool { return symbols[i].ID < symbols[j].ID })
	check := func(id string, match model.Match) {
		if _, err := Matches(code, model.Symbol{}, match); err != nil {
			result.Diagnostics = append(result.Diagnostics, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeInvalidSelector, RuleID: id, Message: err.Error()})
		}
		for _, symbol := range match.Symbols {
			if _, ok := code.Symbol(model.SymbolID(symbol)); !ok {
				result.Diagnostics = append(result.Diagnostics, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeUnresolvedSymbol, RuleID: id, Symbol: model.SymbolID(symbol), Message: "exact symbol does not exist"})
			}
		}
	}
	for _, rule := range rules {
		check(rule.ID, rule.Match)
		if rule.Exclude != nil {
			check(rule.ID, *rule.Exclude)
		}
	}
	for _, exclusion := range exclusions {
		check(exclusion.ID, exclusion.Match)
	}
	if result.Diagnostics.HasErrors() {
		return result
	}
	matchedRules := map[string]bool{}
	for _, symbol := range symbols {
		explanation := Explanation{SymbolID: symbol.ID}
		var candidates []model.Rule
		for _, rule := range rules {
			matched, _ := Matches(code, symbol, rule.Match)
			explanation.Decisions = append(explanation.Decisions, Decision{RuleID: rule.ID, Stage: "include", Matched: matched, Reason: matchReason(matched)})
			if !matched {
				continue
			}
			matchedRules[rule.ID] = true
			if rule.Exclude != nil {
				excluded, _ := Matches(code, symbol, *rule.Exclude)
				explanation.Decisions = append(explanation.Decisions, Decision{RuleID: rule.ID, Stage: "local-exclusion", Matched: excluded, Reason: matchReason(excluded)})
				if excluded {
					result.Plan.Skipped = append(result.Plan.Skipped, model.SkippedTarget{SymbolID: symbol.ID, RuleID: rule.ID, Reason: model.SkipExcluded})
					continue
				}
			}
			candidates = append(candidates, rule)
		}
		excluded := false
		for _, exclusion := range exclusions {
			matched, _ := Matches(code, symbol, exclusion.Match)
			explanation.Decisions = append(explanation.Decisions, Decision{RuleID: exclusion.ID, Stage: "global-exclusion", Matched: matched, Reason: matchReason(matched)})
			if matched && len(candidates) > 0 {
				excluded = true
				result.Plan.Skipped = append(result.Plan.Skipped, model.SkippedTarget{SymbolID: symbol.ID, RuleID: exclusion.ID, Reason: model.SkipExcluded})
			}
		}
		if excluded || len(candidates) == 0 {
			result.Explanations = append(result.Explanations, explanation)
			continue
		}
		var chosen *model.ResolvedTarget
		valid := true
		for _, rule := range candidates {
			target, diags := targetFor(p, rule, symbol)
			result.Diagnostics = append(result.Diagnostics, diags...)
			if diags.HasErrors() {
				valid = false
				continue
			}
			if chosen == nil {
				chosen = &target
				continue
			}
			previousID := chosen.RuleID
			comparable := *chosen
			comparable.RuleID = target.RuleID
			if !reflect.DeepEqual(comparable, target) {
				valid = false
				result.Diagnostics = append(result.Diagnostics, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeConflictingRules, Symbol: symbol.ID, RuleID: rule.ID, Message: fmt.Sprintf("instrumentation conflicts with rule %s", previousID)})
			}
		}
		if valid && chosen != nil {
			result.Plan.Targets = append(result.Plan.Targets, *chosen)
			explanation.Selected = true
		}
		if !valid && len(symbol.ContextIndexes) == 0 && p.Defaults.Context.Mode != model.ContextModeRoot {
			result.Plan.Skipped = append(result.Plan.Skipped, model.SkippedTarget{SymbolID: symbol.ID, RuleID: candidates[0].ID, Reason: model.SkipMissingContext})
		}
		result.Explanations = append(result.Explanations, explanation)
	}
	for _, rule := range rules {
		if !matchedRules[rule.ID] {
			result.Diagnostics = append(result.Diagnostics, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeUnresolvedSymbol, RuleID: rule.ID, Message: "selector matches no symbols"})
		}
	}
	sort.SliceStable(result.Diagnostics, func(i, j int) bool {
		a, b := result.Diagnostics[i], result.Diagnostics[j]
		if a.Symbol != b.Symbol {
			return a.Symbol < b.Symbol
		}
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Message < b.Message
	})
	return result
}

func matchReason(matched bool) string {
	if matched {
		return "selector matched"
	}
	return "selector did not match"
}

func targetFor(p *model.Policy, rule model.Rule, symbol model.Symbol) (model.ResolvedTarget, model.DiagnosticList) {
	target := model.ResolvedTarget{SymbolID: symbol.ID, RuleID: rule.ID, Signature: symbol.Signature}
	var diags model.DiagnosticList
	switch len(symbol.ContextIndexes) {
	case 0:
		if p.Defaults.Context.Mode == model.ContextModeRoot {
			target.ContextStrategy.Strategy = model.ContextStrategyRoot
		} else {
			diags = append(diags, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeMissingContext, RuleID: rule.ID, Symbol: symbol.ID, Message: "selected target has no context.Context argument"})
		}
	case 1:
		target.ContextStrategy = model.ContextStrategy{Strategy: model.ContextStrategyArgument, Index: symbol.ContextIndexes[0]}
	default:
		diags = append(diags, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeMultipleContexts, RuleID: rule.ID, Symbol: symbol.ID, Message: "multiple context.Context arguments require an explicit supported selection strategy"})
	}
	template := p.Defaults.SpanName
	if rule.Span != nil && rule.Span.Name != "" {
		template = rule.Span.Name
	}
	receiver, function, method := "", "", ""
	if symbol.Receiver != nil {
		receiver = symbol.Receiver.Type
	}
	if symbol.Kind == model.SymbolMethod {
		method = symbol.Name
	} else {
		function = symbol.Name
	}
	if template == "" {
		if symbol.Kind == model.SymbolMethod {
			template = "{{package}}.{{receiver}}.{{method}}"
		} else {
			template = "{{package}}.{{function}}"
		}
	}
	name, err := policy.RenderTemplate(template, map[string]string{"symbol": string(symbol.ID), "package": symbol.PackageName, "import_path": symbol.PackageImportPath, "receiver": receiver, "method": method, "function": function})
	if err != nil || name == "" {
		diags = append(diags, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeUnknownTemplateVar, RuleID: rule.ID, Symbol: symbol.ID, Message: "span template must produce a nonempty valid name"})
	}
	target.SpanName = name
	target.ErrorStrategy.Record = p.Defaults.Errors.Record
	if rule.Errors != nil {
		target.ErrorStrategy.Record = rule.Errors.Record
	}
	if target.ErrorStrategy.Record {
		target.ErrorStrategy.Indexes = append([]int(nil), symbol.ErrorIndexes...)
	}
	for _, attr := range rule.Attributes {
		plan := model.AttributePlan{Key: attr.Key, From: attr.From}
		if attr.Safety != nil {
			plan.Classification = attr.Safety.Classification
			plan.Allow = attr.Safety.Allow
		}
		target.Attributes = append(target.Attributes, plan)
	}
	sort.Slice(target.Attributes, func(i, j int) bool { return target.Attributes[i].Key < target.Attributes[j].Key })
	return target, diags
}
