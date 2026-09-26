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
func Resolve(instrumentationPolicy *model.Policy, code *model.CodeModel) Result {
	result := Result{
		Plan: model.ResolvedPlan{
			APIVersion: model.APIVersionV1Alpha1, Targets: []model.ResolvedTarget{}, Skipped: nil,
		},
		Diagnostics: policy.Validate(instrumentationPolicy), Explanations: nil,
	}
	if result.Diagnostics.HasErrors() {
		return result
	}

	if code == nil {
		result.Diagnostics = append(result.Diagnostics,
			resolutionDiagnostic(model.CodeInvalidPolicy, "", "", "code model is required"),
		)

		return result
	}

	rules := append([]model.Rule(nil), instrumentationPolicy.Rules...)
	exclusions := append([]model.Exclusion(nil), instrumentationPolicy.Exclusions...)
	symbols := append([]model.Symbol(nil), code.Symbols...)

	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	sort.Slice(exclusions, func(i, j int) bool { return exclusions[i].ID < exclusions[j].ID })
	sort.Slice(symbols, func(i, j int) bool { return symbols[i].ID < symbols[j].ID })

	for _, rule := range rules {
		result.Diagnostics = append(result.Diagnostics, validateSelector(code, rule.ID, rule.Match)...)

		if rule.Exclude != nil {
			result.Diagnostics = append(result.Diagnostics, validateSelector(code, rule.ID, *rule.Exclude)...)
		}
	}

	for _, exclusion := range exclusions {
		result.Diagnostics = append(result.Diagnostics, validateSelector(code, exclusion.ID, exclusion.Match)...)
	}

	if result.Diagnostics.HasErrors() {
		return result
	}

	matchedRules := result.resolveSymbols(instrumentationPolicy, code, symbols, rules, exclusions)

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
		return diagnosticLess(result.Diagnostics[i], result.Diagnostics[j])
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

func validateSelector(code *model.CodeModel, ruleID string, match model.Match) model.DiagnosticErrorList {
	var diagnostics model.DiagnosticErrorList

	var symbol model.Symbol

	_, err := Matches(code, symbol, match)
	if err != nil {
		diagnostics = append(diagnostics, resolutionDiagnostic(
			model.CodeInvalidSelector,
			ruleID,
			"",
			err.Error(),
		))
	}

	for _, symbolID := range match.Symbols {
		_, ok := code.Symbol(model.SymbolID(symbolID))
		if !ok {
			diagnostics = append(diagnostics, resolutionDiagnostic(
				model.CodeUnresolvedSymbol,
				ruleID,
				model.SymbolID(symbolID),
				"exact symbol does not exist",
			))
		}
	}

	return diagnostics
}

func chooseTarget(
	instrumentationPolicy *model.Policy, symbol model.Symbol, candidates []model.Rule,
) (*model.ResolvedTarget, model.DiagnosticErrorList) {
	var diagnostics model.DiagnosticErrorList

	var chosen *model.ResolvedTarget

	valid := true

	for _, rule := range candidates {
		target, diags := targetFor(instrumentationPolicy, rule, symbol)

		diagnostics = append(diagnostics, diags...)

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

			diagnostics = append(diagnostics, resolutionDiagnostic(
				model.CodeConflictingRules,
				rule.ID,
				symbol.ID,
				"instrumentation conflicts with rule "+previousID,
			))
		}
	}

	if !valid {
		return nil, diagnostics
	}

	return chosen, diagnostics
}

func (result *Result) candidateRules(
	code *model.CodeModel, symbol model.Symbol, rules []model.Rule,
	explanation *Explanation, matchedRules map[string]bool,
) []model.Rule {
	var candidates []model.Rule

	for _, rule := range rules {
		matched, _ := Matches(code, symbol, rule.Match)

		explanation.Decisions = append(explanation.Decisions, Decision{
			RuleID: rule.ID, Stage: "include", Matched: matched, Reason: matchReason(matched),
		})

		if !matched {
			continue
		}

		matchedRules[rule.ID] = true

		if rule.Exclude != nil {
			excluded, _ := Matches(code, symbol, *rule.Exclude)

			explanation.Decisions = append(explanation.Decisions, Decision{
				RuleID: rule.ID, Stage: "local-exclusion", Matched: excluded, Reason: matchReason(excluded),
			})

			if excluded {
				result.Plan.Skipped = append(result.Plan.Skipped, model.SkippedTarget{
					SymbolID: symbol.ID, RuleID: rule.ID, Reason: model.SkipExcluded,
				})

				continue
			}
		}

		candidates = append(candidates, rule)
	}

	return candidates
}

func diagnosticLess(first, second model.DiagnosticError) bool {
	if first.Symbol != second.Symbol {
		return first.Symbol < second.Symbol
	}

	if first.RuleID != second.RuleID {
		return first.RuleID < second.RuleID
	}

	if first.Code != second.Code {
		return first.Code < second.Code
	}

	return first.Message < second.Message
}

func (result *Result) resolveSymbols(
	instrumentationPolicy *model.Policy, code *model.CodeModel, symbols []model.Symbol,
	rules []model.Rule, exclusions []model.Exclusion,
) map[string]bool {
	matchedRules := map[string]bool{}

	for _, symbol := range symbols {
		explanation := Explanation{SymbolID: symbol.ID, Decisions: nil, Selected: false}

		candidates := result.candidateRules(code, symbol, rules, &explanation, matchedRules)

		excluded := result.applyGlobalExclusions(code, symbol, exclusions, len(candidates), &explanation)

		if excluded || len(candidates) == 0 {
			result.Explanations = append(result.Explanations, explanation)

			continue
		}

		chosen, diagnostics := chooseTarget(instrumentationPolicy, symbol, candidates)
		result.Diagnostics = append(result.Diagnostics, diagnostics...)

		if chosen != nil {
			result.Plan.Targets = append(result.Plan.Targets, *chosen)
			explanation.Selected = true
		}

		if diagnostics.HasErrors() && len(symbol.ContextIndexes) == 0 &&
			instrumentationPolicy.Defaults.Context.Mode != model.ContextModeRoot {
			result.Plan.Skipped = append(result.Plan.Skipped, model.SkippedTarget{
				SymbolID: symbol.ID, RuleID: candidates[0].ID, Reason: model.SkipMissingContext,
			})
		}

		result.Explanations = append(result.Explanations, explanation)
	}

	return matchedRules
}

func (result *Result) applyGlobalExclusions(
	code *model.CodeModel, symbol model.Symbol, exclusions []model.Exclusion,
	candidateCount int, explanation *Explanation,
) bool {
	excluded := false

	for _, exclusion := range exclusions {
		matched, _ := Matches(code, symbol, exclusion.Match)

		explanation.Decisions = append(explanation.Decisions, Decision{
			RuleID: exclusion.ID, Stage: "global-exclusion", Matched: matched, Reason: matchReason(matched),
		})

		if matched && candidateCount > 0 {
			excluded = true

			result.Plan.Skipped = append(result.Plan.Skipped, model.SkippedTarget{
				SymbolID: symbol.ID, RuleID: exclusion.ID, Reason: model.SkipExcluded,
			})
		}
	}

	return excluded
}
