package resolve

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/DiLRandI/OTelPlan/internal/policy"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

// Decision records whether one rule or exclusion matched at a selection stage.
type Decision struct {
	RuleID  string `json:"rule"`
	Stage   string `json:"stage"`
	Matched bool   `json:"matched"`
	Reason  string `json:"reason"`
}

// Explanation records selection decisions for one symbol, including rejected candidates.
type Explanation struct {
	SymbolID  model.SymbolID `json:"symbol"`
	Decisions []Decision     `json:"decisions"`
	Selected  bool           `json:"selected"`
}

// Result contains the resolved plan, validation diagnostics, and per-symbol decisions.
// Callers must check Diagnostics.HasErrors before using the plan for instrumentation.
type Result struct {
	Plan         model.ResolvedPlan        `json:"plan"`
	Diagnostics  model.DiagnosticErrorList `json:"diagnostics"`
	Explanations []Explanation             `json:"explanations"`
}

// Resolve applies a policy without mutating it or the code model.
// Rule, exclusion, and symbol declaration order does not affect the result.
// Invalid policies and missing code models produce error diagnostics.
func Resolve(p *model.Policy, code *model.CodeModel) Result {
	result := Result{Plan: model.ResolvedPlan{APIVersion: model.APIVersionV1Alpha1, Targets: []model.ResolvedTarget{}}, Diagnostics: policy.Validate(p)}
	if result.Diagnostics.HasErrors() {
		return result
	}

	if code == nil {
		result.Diagnostics = append(result.Diagnostics, resolutionDiagnostic(
			model.CodeInvalidPolicy,
			"",
			"",
			"code model is required",
		))

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
			result.Diagnostics = append(result.Diagnostics, resolutionDiagnostic(
				model.CodeInvalidSelector,
				id,
				"",
				err.Error(),
			))
		}

		for _, symbol := range match.Symbols {
			if _, ok := code.Symbol(model.SymbolID(symbol)); !ok {
				result.Diagnostics = append(result.Diagnostics, resolutionDiagnostic(
					model.CodeUnresolvedSymbol,
					id,
					model.SymbolID(symbol),
					"exact symbol does not exist",
				))
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
			chosenRule := *chosen

			chosenRule.RuleID = target.RuleID

			if !reflect.DeepEqual(chosenRule, target) {
				valid = false

				result.Diagnostics = append(result.Diagnostics, resolutionDiagnostic(
					model.CodeConflictingRules,
					rule.ID,
					symbol.ID,
					"instrumentation conflicts with rule "+previousID,
				))
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
			result.Diagnostics = append(result.Diagnostics, resolutionDiagnostic(
				model.CodeUnresolvedSymbol,
				rule.ID,
				"",
				"selector matches no symbols",
			))
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

func targetFor(
	instrumentationPolicy *model.Policy, rule model.Rule, symbol model.Symbol,
) (model.ResolvedTarget, model.DiagnosticErrorList) {
	contextStrategy, diagnostics := targetContext(instrumentationPolicy.Defaults.Context.Mode, rule.ID, symbol)

	name, err := targetSpanName(instrumentationPolicy.Defaults.SpanName, rule.Span, symbol)
	if err != nil || name == "" {
		diagnostics = append(diagnostics, resolutionDiagnostic(
			model.CodeUnknownTemplateVar, rule.ID, symbol.ID,
			"span template must produce a nonempty valid name",
		))
	}

	target := model.ResolvedTarget{
		SymbolID:        symbol.ID,
		SpanName:        name,
		ContextStrategy: contextStrategy,
		ErrorStrategy:   targetErrors(instrumentationPolicy.Defaults.Errors.Record, rule.Errors, symbol.ErrorIndexes),
		Attributes:      targetAttributes(rule.Attributes),
		RuleID:          rule.ID,
		Signature:       symbol.Signature,
	}

	return target, diagnostics
}

func targetContext(
	mode model.ContextMode, ruleID string, symbol model.Symbol,
) (model.ContextStrategy, model.DiagnosticErrorList) {
	var strategy model.ContextStrategy

	switch len(symbol.ContextIndexes) {
	case 0:
		if mode == model.ContextModeRoot {
			return model.ContextStrategy{Strategy: model.ContextStrategyRoot, Index: 0}, nil
		}

		return strategy, model.DiagnosticErrorList{resolutionDiagnostic(
			model.CodeMissingContext, ruleID, symbol.ID,
			"selected target has no context.Context argument",
		)}
	case 1:
		return model.ContextStrategy{
			Strategy: model.ContextStrategyArgument, Index: symbol.ContextIndexes[0],
		}, nil
	default:
		return strategy, model.DiagnosticErrorList{resolutionDiagnostic(
			model.CodeMultipleContexts, ruleID, symbol.ID,
			"multiple context.Context arguments require an explicit supported selection strategy",
		)}
	}
}

func targetSpanName(template string, span *model.SpanConfig, symbol model.Symbol) (string, error) {
	if span != nil && span.Name != "" {
		template = span.Name
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

	name, err := policy.RenderTemplate(template, map[string]string{
		"symbol": string(symbol.ID), "package": symbol.PackageName, "import_path": symbol.PackageImportPath,
		"receiver": receiver, "method": method, "function": function,
	})
	if err != nil {
		return name, fmt.Errorf("render target span name: %w", err)
	}

	return name, nil
}

func targetErrors(record bool, override *model.ErrorConfig, indexes []int) model.ErrorStrategy {
	strategy := model.ErrorStrategy{Record: record, Indexes: nil}
	if override != nil {
		strategy.Record = override.Record
	}

	if strategy.Record {
		strategy.Indexes = append([]int(nil), indexes...)
	}

	return strategy
}

func targetAttributes(attributes []model.AttributeRule) []model.AttributePlan {
	if len(attributes) == 0 {
		return nil
	}

	plans := make([]model.AttributePlan, 0, len(attributes))

	for _, attribute := range attributes {
		plan := model.AttributePlan{Key: attribute.Key, From: attribute.From, Classification: "", Allow: false}
		if attribute.Safety != nil {
			plan.Classification = attribute.Safety.Classification
			plan.Allow = attribute.Safety.Allow
		}

		plans = append(plans, plan)
	}

	sort.Slice(plans, func(i, j int) bool { return plans[i].Key < plans[j].Key })

	return plans
}

func resolutionDiagnostic(
	code model.Code, ruleID string, symbolID model.SymbolID, message string,
) model.DiagnosticError {
	return model.DiagnosticError{
		Severity: model.SeverityError,
		Code:     code,
		Message:  message,
		RuleID:   ruleID,
		Symbol:   symbolID,
		File:     "",
		Line:     0,
	}
}
