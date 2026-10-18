package otelc_test

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestRecoverShadowingRejectsOnlySelectedAffectedTargets(t *testing.T) {
	t.Parallel()

	code, plan := hookFixture()
	code.Symbols[0].RecoverShadowed = true
	plan.Targets = plan.Targets[:1]

	diagnostics := otelc.Check(otelc.SupportedVersion, code, plan)
	if !diagnostics.HasErrors() {
		t.Fatal("pinned backend accepted a shadowed recover binding")
	}

	if len(diagnostics) != 1 || diagnostics[0].Code != model.CodeBackendUnsupported ||
		diagnostics[0].Symbol != plan.Targets[0].SymbolID || diagnostics[0].RuleID != plan.Targets[0].RuleID {
		t.Fatalf("backend lost rejection provenance: %+v", diagnostics)
	}

	code.Symbols[0].RecoverShadowed = false

	var unrelated model.Symbol

	unrelated.ID, unrelated.RecoverShadowed = "example.com/other.Other", true

	code.Symbols = append(code.Symbols, unrelated)
	if diagnostics := otelc.Check(otelc.SupportedVersion, code, plan); diagnostics.HasErrors() {
		t.Fatal("unselected shadowed package poisoned plan")
	}
}
