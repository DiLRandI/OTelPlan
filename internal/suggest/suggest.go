// Package suggest ranks advisory instrumentation candidates from the code model.
package suggest

import (
	"fmt"
	"slices"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const (
	contextScore          = 1
	exportedScore         = 1
	errorScore            = 2
	interfaceScore        = 2
	staticCallerScore     = 2
	conservativeScore     = 1
	infrastructureScore   = 2
	nearEntrypointScore   = 3
	entrypointScore       = 2
	distantEntrypoint     = 1
	lowSignalPenalty      = 2
	highConfidenceScore   = 8
	mediumConfidenceScore = 5
	nearEntrypointDepth   = 3
	mainPackageName       = "main"
)

// Suggestion describes one advisory candidate and the evidence behind its rank.
type Suggestion struct {
	SymbolID   model.SymbolID `json:"symbol"`
	Score      int            `json:"score"`
	Confidence string         `json:"confidence"`
	Evidence   []string       `json:"evidence"`
}

type callSignals struct {
	staticCaller       bool
	conservativeCaller bool
	infrastructure     bool
	entrypointDepth    int
}

// Rank returns conservative, deterministic suggestions without selecting policy targets.
func Rank(code *model.CodeModel) []Suggestion {
	if code == nil {
		return nil
	}

	symbols := make(map[model.SymbolID]model.Symbol, len(code.Symbols))
	for _, symbol := range code.Symbols {
		symbols[symbol.ID] = symbol
	}

	interfaces := make(map[model.SymbolID]bool, len(code.InterfaceMethods))
	for _, relation := range code.InterfaceMethods {
		interfaces[relation.SymbolID] = true
	}

	calls := collectCallSignals(code, symbols)

	suggestions := make([]Suggestion, 0, len(code.Symbols))
	for _, symbol := range code.Symbols {
		if !eligible(symbol) {
			continue
		}

		suggestions = append(suggestions, rankSymbol(symbol, interfaces[symbol.ID], calls[symbol.ID]))
	}

	slices.SortFunc(suggestions, func(left, right Suggestion) int {
		if left.Score != right.Score {
			return right.Score - left.Score
		}

		return strings.Compare(string(left.SymbolID), string(right.SymbolID))
	})

	return suggestions
}

func eligible(symbol model.Symbol) bool {
	return symbol.Ownership == model.OwnershipApplication && symbol.HasBody && !symbol.Generated && !symbol.TestFile &&
		symbol.PackageName != mainPackageName && !symbol.Variadic && len(symbol.ContextIndexes) == 1 &&
		(symbol.Generics == nil || len(symbol.Generics.TypeParams) == 0)
}

func rankSymbol(symbol model.Symbol, implementsInterface bool, calls callSignals) Suggestion {
	suggestion := Suggestion{
		SymbolID: symbol.ID, Score: contextScore, Confidence: "low",
		Evidence: []string{"has one context.Context argument"},
	}
	addStructuralEvidence(&suggestion, symbol, implementsInterface)
	addCallEvidence(&suggestion, calls)

	if len(symbol.ErrorIndexes) == 0 && !implementsInterface && !calls.staticCaller &&
		!calls.infrastructure && calls.entrypointDepth == 0 {
		suggestion.Score -= lowSignalPenalty
		suggestion.Evidence = append(suggestion.Evidence, "low signal: no error, interface boundary, or static call path")
	}

	if suggestion.Score >= highConfidenceScore {
		suggestion.Confidence = "high"
	} else if suggestion.Score >= mediumConfidenceScore {
		suggestion.Confidence = "medium"
	}

	return suggestion
}

func addStructuralEvidence(suggestion *Suggestion, symbol model.Symbol, implementsInterface bool) {
	if symbol.Visibility == model.VisibilityExported {
		suggestion.Score += exportedScore
		suggestion.Evidence = append(suggestion.Evidence, "exported declaration")
	}

	if len(symbol.ErrorIndexes) > 0 {
		suggestion.Score += errorScore
		suggestion.Evidence = append(suggestion.Evidence, "returns an error")
	}

	if implementsInterface {
		suggestion.Score += interfaceScore
		suggestion.Evidence = append(suggestion.Evidence, "implements an interface method")
	}
}

func addCallEvidence(suggestion *Suggestion, calls callSignals) {
	if calls.staticCaller {
		suggestion.Score += staticCallerScore
		suggestion.Evidence = append(suggestion.Evidence, "has a static application caller")
	} else if calls.conservativeCaller {
		suggestion.Score += conservativeScore
		suggestion.Evidence = append(suggestion.Evidence, "has a conservative application caller candidate")
	}

	if calls.infrastructure {
		suggestion.Score += infrastructureScore
		suggestion.Evidence = append(suggestion.Evidence, "calls a known infrastructure API")
	}

	if calls.entrypointDepth > 0 {
		suggestion.Score += entrypointWeight(calls.entrypointDepth)
		suggestion.Evidence = append(suggestion.Evidence,
			fmt.Sprintf("reachable from main.main through %d static call(s)", calls.entrypointDepth))
	}
}

func entrypointWeight(depth int) int {
	if depth == 1 {
		return nearEntrypointScore
	}

	if depth <= nearEntrypointDepth {
		return entrypointScore
	}

	return distantEntrypoint
}

func collectCallSignals(code *model.CodeModel, symbols map[model.SymbolID]model.Symbol) map[model.SymbolID]callSignals {
	signals := map[model.SymbolID]callSignals{}
	if code.CallGraph == nil {
		return signals
	}

	static := map[model.SymbolID][]model.SymbolID{}
	seen := map[model.CallRelation]bool{}

	for _, edge := range code.CallEdges {
		if seen[edge] {
			continue
		}

		seen[edge] = true

		caller, known := symbols[edge.Caller]
		if !known || caller.Ownership != model.OwnershipApplication {
			continue
		}

		recordCallSignal(edge, symbols, signals, static)
	}

	for id, depth := range entrypointDepths(symbols, static) {
		signal := signals[id]
		signal.entrypointDepth = depth
		signals[id] = signal
	}

	return signals
}

func recordCallSignal(edge model.CallRelation, symbols map[model.SymbolID]model.Symbol,
	signals map[model.SymbolID]callSignals, static map[model.SymbolID][]model.SymbolID,
) {
	if _, known := symbols[edge.Callee]; known && edge.Caller != edge.Callee {
		signal := signals[edge.Callee]
		if edge.Precision == model.CallPrecisionStatic {
			signal.staticCaller = true

			static[edge.Caller] = append(static[edge.Caller], edge.Callee)
		} else {
			signal.conservativeCaller = true
		}

		signals[edge.Callee] = signal
	}

	if edge.Precision == model.CallPrecisionStatic && infrastructureCall(edge.Callee) {
		signal := signals[edge.Caller]
		signal.infrastructure = true
		signals[edge.Caller] = signal
	}
}

func entrypointDepths(symbols map[model.SymbolID]model.Symbol,
	calls map[model.SymbolID][]model.SymbolID,
) map[model.SymbolID]int {
	depths := map[model.SymbolID]int{}
	queue := []model.SymbolID{}

	for _, symbol := range symbols {
		if isMainEntrypoint(symbol) {
			queue = append(queue, symbol.ID)
			depths[symbol.ID] = 0
		}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, callee := range calls[current] {
			if _, visited := depths[callee]; visited {
				continue
			}

			depths[callee] = depths[current] + 1
			queue = append(queue, callee)
		}
	}

	return depths
}

func isMainEntrypoint(symbol model.Symbol) bool {
	return symbol.PackageName == mainPackageName && symbol.Kind == model.SymbolFunction &&
		symbol.Name == mainPackageName && symbol.Ownership == model.OwnershipApplication
}

func infrastructureCall(id model.SymbolID) bool {
	parsed, err := model.ParseSymbolID(id)
	if err != nil {
		return false
	}

	return parsed.ImportPath == "database/sql" || parsed.ImportPath == "net/http"
}
