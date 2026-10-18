package validate_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestSafetyTargetLimitDefaultsAndBoundaries(t *testing.T) {
	t.Parallel()

	checks := []struct {
		count    int
		warnings int
		err      bool
	}{
		{100, 0, false}, {101, 1, false}, {500, 1, false},
		{501, 2, false}, {2000, 2, false}, {2001, 2, true},
	}
	for _, check := range checks {
		code, plan := targetLimitPlan(check.count)

		var options validate.Options

		diagnostics := validate.Safety(code, plan, options)
		if countBroadWarnings(diagnostics) != check.warnings || diagnostics.HasErrors() != check.err {
			t.Fatalf("count=%d diagnostics=%+v", check.count, diagnostics)
		}
	}
}

func TestSafetyTargetLimitConfiguredThresholds(t *testing.T) {
	t.Parallel()

	var options validate.Options

	options.WarningTargets, options.StrongWarningTargets, options.MaximumTargets = 2, 4, 3
	options.EnforceMaximum = true

	checks := []struct {
		count    int
		warnings int
		err      bool
	}{
		{2, 0, false}, {3, 1, false}, {4, 1, true}, {5, 2, true},
	}
	for _, check := range checks {
		code, plan := targetLimitPlan(check.count)

		for _, acknowledged := range []bool{false, true} {
			options.AllowLargePlan = acknowledged

			diagnostics := validate.Safety(code, plan, options)
			if countBroadWarnings(diagnostics) != check.warnings || diagnostics.HasErrors() != check.err {
				t.Fatalf("count=%d acknowledged=%t diagnostics=%+v", check.count, acknowledged, diagnostics)
			}
		}
	}
}

func TestSafetyTargetLimitLegacyAcknowledgmentAndImmutability(t *testing.T) {
	t.Parallel()

	code, plan := targetLimitPlan(2001)

	var options validate.Options

	options.AllowLargePlan = true
	options.DenyPatterns = []string{"private"}
	beforeCode, beforePlan := targetLimitJSON(t, code), targetLimitJSON(t, plan)
	beforeOptions := targetLimitJSON(t, options)

	diagnostics := validate.Safety(code, plan, options)
	if countBroadWarnings(diagnostics) != 2 || diagnostics.HasErrors() {
		t.Fatalf("legacy acknowledgment changed: %+v", diagnostics)
	}

	if !bytes.Equal(beforeCode, targetLimitJSON(t, code)) || !bytes.Equal(beforePlan, targetLimitJSON(t, plan)) ||
		!bytes.Equal(beforeOptions, targetLimitJSON(t, options)) {
		t.Fatal("safety thresholds mutated caller inputs")
	}
}

func targetLimitJSON(t *testing.T, value any) []byte {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func targetLimitPlan(count int) (*model.CodeModel, model.ResolvedPlan) {
	var symbol model.Symbol

	symbol.ID = "example.com/app.Run"

	var code model.CodeModel

	code.Symbols = []model.Symbol{symbol}

	var target model.ResolvedTarget

	target.SymbolID = symbol.ID
	target.RuleID = "operation"

	var plan model.ResolvedPlan

	plan.Targets = make([]model.ResolvedTarget, count)
	for index := range plan.Targets {
		plan.Targets[index] = target
	}

	return &code, plan
}

func countBroadWarnings(diagnostics model.DiagnosticErrorList) int {
	count := 0

	for _, diagnostic := range diagnostics {
		if diagnostic.Code == model.CodeBroadPlan && diagnostic.Severity == model.SeverityWarning {
			count++
		}
	}

	return count
}
