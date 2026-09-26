package resolve_test

import (
	"errors"
	"path"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/resolve"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestMatchesValidatesAllPatterns(t *testing.T) {
	t.Parallel()

	var symbol model.Symbol

	symbol.Kind = model.SymbolFunction
	symbol.Name = "Run"
	symbol.PackageImportPath = "example.com/app"

	var code model.CodeModel

	for _, earlierMatch := range []bool{false, true} {
		name := "earlier mismatch"
		if earlierMatch {
			name = "earlier match"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var selector model.Match

			selector.Packages = []string{"different"}
			if earlierMatch {
				selector.Packages = []string{"example.com/app"}
			}

			selector.Functions = []string{"Run", "["}

			matched, err := resolve.Matches(&code, symbol, selector)
			if matched || !errors.Is(err, path.ErrBadPattern) {
				t.Fatalf("Matches = %v, %v; want invalid-pattern error despite earlier result", matched, err)
			}
		})
	}
}
