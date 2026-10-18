package lockfile_test

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestStringerLockTracksConversionForStringLikeSources(t *testing.T) {
	t.Parallel()

	policy, code, plan := creationInputs()

	var parameter model.Parameter

	parameter.Name, parameter.Type = "value", "example.com/app.Category"
	code.Symbols[0].Parameters = []model.Parameter{parameter}

	var typ model.TypeInfo

	typ.Type, typ.Kind, typ.Stringer = parameter.Type, "string", true
	code.Types = []model.TypeInfo{typ}

	var attribute model.AttributePlan

	attribute.Key, attribute.From.Argument = "category", "value"
	plan.Targets[0].Attributes = []model.AttributePlan{attribute}
	plain := createMetadataLock(t, policy, code, plan)
	plan.Targets[0].Attributes[0].Stringer = true

	formatted := createMetadataLock(t, policy, code, plan)
	if !formatted.Targets[0].Attributes[0].Stringer || plain.Targets[0].Attributes[0].Stringer {
		t.Fatal("conversion intent lost")
	}

	diff := lockfile.Diff(plain, formatted)
	if len(diff.Entries) != 1 || diff.Entries[0].Classification != model.DiffAttribute {
		t.Fatalf("conversion did not cause attribute drift: %+v", diff)
	}

	encoded, err := lockfile.Marshal(formatted)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := lockfile.Parse(encoded)
	if err != nil || !lockfile.Diff(formatted, parsed).Empty() {
		t.Fatalf("Stringer lock did not roundtrip: %v", err)
	}

	for _, mutate := range []func(*model.LockAttribute){
		func(a *model.LockAttribute) { a.Kind = "integer" },
		func(a *model.LockAttribute) { a.From.Result = "result" },
		func(a *model.LockAttribute) { a.From.Constant = "private-marker" },
		func(a *model.LockAttribute) { a.From.Symbol = "name" },
		func(a *model.LockAttribute) { a.From.ErrorType = "0" },
		func(a *model.LockAttribute) { a.From.Argument = "" },
	} {
		assertMalformedMetadataLock(t, encoded, mutate)
	}
}
