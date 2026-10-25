package suggest

import (
	"fmt"
	"slices"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

type entrypointPath struct {
	depth  int
	source string
}

func entrypointPaths(symbols map[model.SymbolID]model.Symbol,
	calls map[model.SymbolID][]model.SymbolID,
) map[model.SymbolID]entrypointPath {
	paths := map[model.SymbolID]entrypointPath{}

	var queue []model.SymbolID

	for _, symbol := range symbols {
		source := entrypointSource(symbol)
		if source != "" {
			queue = append(queue, symbol.ID)
			paths[symbol.ID] = entrypointPath{depth: 0, source: source}
		}
	}

	slices.Sort(queue)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, callee := range calls[current] {
			if _, visited := paths[callee]; visited {
				continue
			}

			paths[callee] = entrypointPath{depth: paths[current].depth + 1, source: paths[current].source}
			queue = append(queue, callee)
		}
	}

	return paths
}

func entrypointSource(symbol model.Symbol) string {
	if isMainEntrypoint(symbol) {
		return "main.main"
	}

	if symbol.Ownership != model.OwnershipApplication || len(symbol.Entrypoints) == 0 {
		return ""
	}

	integrations := slices.Clone(symbol.Entrypoints)
	slices.Sort(integrations)
	integrations = slices.Compact(integrations)

	return fmt.Sprintf("%s (%s)", symbol.ID, strings.Join(integrations, ", "))
}
