package validate_test

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestSafetyWarnsOnlyForSelectedRecursiveTargets(t *testing.T) {
	t.Parallel()

	var recursive model.Symbol

	recursive.ID, recursive.Recursive = "example.com/recursion.Direct", true

	var nonrecursive model.Symbol

	nonrecursive.ID = "example.com/recursion.Wrapper"

	var code model.CodeModel

	code.Symbols = []model.Symbol{recursive, nonrecursive}

	var recursiveTarget, ordinaryTarget model.ResolvedTarget

	recursiveTarget.SymbolID, recursiveTarget.RuleID = recursive.ID, "recursive-rule"
	ordinaryTarget.SymbolID, ordinaryTarget.RuleID = nonrecursive.ID, "ordinary-rule"

	var plan model.ResolvedPlan

	plan.Targets = []model.ResolvedTarget{recursiveTarget, ordinaryTarget}

	var options validate.Options

	diagnostics := validate.Safety(&code, plan, options)
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics=%+v, want one recursion warning", diagnostics)
	}

	warning := diagnostics[0]
	if warning.Severity != model.SeverityWarning || warning.Code != model.CodeRecursiveTarget ||
		warning.RuleID != "recursive-rule" || warning.Symbol != recursive.ID {
		t.Fatalf("unexpected recursion warning: %+v", warning)
	}

	plan.Targets = []model.ResolvedTarget{ordinaryTarget}
	if diagnostics := validate.Safety(&code, plan, options); len(diagnostics) != 0 {
		t.Fatalf("unselected recursive cycle produced diagnostics: %+v", diagnostics)
	}
}
