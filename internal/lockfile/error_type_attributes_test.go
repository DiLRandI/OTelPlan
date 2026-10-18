package lockfile_test

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestErrorTypeLockRoundTripAndSourceDrift(t *testing.T) {
	t.Parallel()

	policy, code, plan := creationInputs()

	var first, second model.Result

	first.Name, first.Type = "first", "error"
	second.Name, second.Type = "second", "error"
	code.Symbols[0].Results, code.Symbols[0].ErrorIndexes = []model.Result{first, second}, []int{0, 1}

	var typ model.TypeInfo

	typ.Type, typ.Expression, typ.Kind = "error", "error", "interface"
	code.Types = []model.TypeInfo{typ}

	var attribute model.AttributePlan

	attribute.Key, attribute.From.ErrorType = "failure.class", "first"
	plan.Targets[0].Attributes = []model.AttributePlan{attribute}
	firstLock := createMetadataLock(t, policy, code, plan)

	encoded, err := lockfile.Marshal(firstLock)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := lockfile.Parse(encoded)
	if err != nil || !lockfile.Diff(firstLock, parsed).Empty() {
		t.Fatalf("error type lock failed to roundtrip: %v", err)
	}

	plan.Targets[0].Attributes[0].From.ErrorType = "second"
	secondLock := createMetadataLock(t, policy, code, plan)

	diff := lockfile.Diff(firstLock, secondLock)
	if len(diff.Entries) != 1 || diff.Entries[0].Classification != model.DiffAttribute {
		t.Fatalf("error type source drift: %+v", diff)
	}

	for _, mutate := range []func(*model.LockAttribute){
		func(a *model.LockAttribute) { a.Kind = "integer" },
		func(a *model.LockAttribute) { a.From.Argument = "private-marker" },
		func(a *model.LockAttribute) { a.From.Constant = "private-marker" },
		func(a *model.LockAttribute) { a.From.Result = "private-marker" },
		func(a *model.LockAttribute) { a.From.Symbol = "name" },
		func(a *model.LockAttribute) { value := "private-marker"; a.SymbolValue = &value },
		func(a *model.LockAttribute) { a.From.ErrorType = "private-marker.Field" },
		func(a *model.LockAttribute) { a.From.ErrorType = "-1" },
		func(a *model.LockAttribute) { a.From.ErrorType = "_" },
		func(a *model.LockAttribute) { a.From.ErrorType = "9999999999999999999999999" },
	} {
		assertMalformedMetadataLock(t, encoded, mutate)
	}
}
