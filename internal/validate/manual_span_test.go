package validate_test

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestSafetyWarnsOnlyForSelectedManualSpanTargets(t *testing.T) {
	t.Parallel()

	var manual, ordinary model.Symbol

	manual.ID, manual.ManualSpan = "example.com/manual.Direct", true
	ordinary.ID = "example.com/manual.Helper"

	var code model.CodeModel

	code.Symbols = []model.Symbol{manual, ordinary}

	var manualTarget, ordinaryTarget model.ResolvedTarget

	manualTarget.SymbolID, manualTarget.RuleID = manual.ID, "manual-rule"
	ordinaryTarget.SymbolID, ordinaryTarget.RuleID = ordinary.ID, "ordinary-rule"

	var plan model.ResolvedPlan

	plan.Targets = []model.ResolvedTarget{manualTarget, ordinaryTarget}

	var options validate.Options

	diagnostics := validate.Safety(&code, plan, options)
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics=%+v, want one manual span warning", diagnostics)
	}

	warning := diagnostics[0]
	if warning.Severity != model.SeverityWarning || warning.Code != model.CodeManualSpan ||
		warning.RuleID != "manual-rule" || warning.Symbol != manual.ID {
		t.Fatalf("unexpected manual span warning: %+v", warning)
	}

	plan.Targets = []model.ResolvedTarget{ordinaryTarget}
	if diagnostics := validate.Safety(&code, plan, options); len(diagnostics) != 0 {
		t.Fatalf("unselected manual span produced diagnostics: %+v", diagnostics)
	}
}
