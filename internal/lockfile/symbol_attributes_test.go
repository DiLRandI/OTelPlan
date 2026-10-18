package lockfile_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestSymbolMetadataLockFreezesValuesAndDetectsDrift(t *testing.T) {
	t.Parallel()

	policy, code, plan := creationInputs()
	code.Symbols[0].PackageName = "original"
	plan.Targets[0].SpanName = "fixed"

	var attribute model.AttributePlan

	attribute.Key, attribute.From.Symbol = "code.package", "packageName"
	plan.Targets[0].Attributes = []model.AttributePlan{attribute}

	first := createMetadataLock(t, policy, code, plan)

	value := first.Targets[0].Attributes[0].SymbolValue
	if value == nil || *value != "original" {
		t.Fatalf("metadata was not frozen: %+v", first.Targets[0].Attributes)
	}

	code.Symbols[0].PackageName = "changed"
	second := createMetadataLock(t, policy, code, plan)

	diff := lockfile.Diff(first, second)
	if len(diff.Entries) != 1 || diff.Entries[0].Classification != model.DiffAttribute {
		t.Fatalf("metadata change with fixed span name did not produce ATTRIBUTE drift: %+v", diff)
	}

	if *first.Targets[0].Attributes[0].SymbolValue != "original" {
		t.Fatal("refreshing metadata changed a caller-owned lock")
	}

	code.ModuleRoot = "/relocated/checkout"
	code.Symbols[0].Location.Line, code.Symbols[0].Location.Column = 100, 20

	relocated := createMetadataLock(t, policy, code, plan)
	if !lockfile.Diff(second, relocated).Empty() {
		t.Fatal("checkout relocation or source position changed symbol metadata identity")
	}

	encoded, err := lockfile.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := lockfile.Parse(encoded)
	if err != nil || !lockfile.Diff(first, parsed).Empty() {
		t.Fatalf("metadata lock did not roundtrip: %v", err)
	}
}

func createMetadataLock(t *testing.T, policy *model.Policy, code *model.CodeModel,
	plan model.ResolvedPlan) model.Lockfile {
	t.Helper()

	var backend model.LockBackend

	backend.Name, backend.Version = policy.Backend.Name, policy.Backend.Version

	locked, err := lockfile.Create(policy, code, plan, backend, lockfile.Digest(nil), nil)
	if err != nil {
		t.Fatal(err)
	}

	return locked
}

func TestMalformedSymbolMetadataLocksFailWithoutValues(t *testing.T) {
	t.Parallel()

	policy, code, plan := creationInputs()

	var attribute model.AttributePlan

	attribute.Key, attribute.From.Symbol = "code.receiver", "receiver"
	plan.Targets[0].Attributes = []model.AttributePlan{attribute}
	locked := createMetadataLock(t, policy, code, plan)

	value := locked.Targets[0].Attributes[0].SymbolValue
	if value == nil || *value != "" {
		t.Fatal("empty function receiver must be frozen distinctly from missing metadata")
	}

	encoded, err := json.Marshal(locked)
	if err != nil {
		t.Fatal(err)
	}

	for _, mutate := range []func(*model.LockAttribute){
		func(attr *model.LockAttribute) { attr.SymbolValue = nil },
		func(attr *model.LockAttribute) { attr.From.Symbol = "private-marker" },
		func(attr *model.LockAttribute) { attr.From.Argument = "private-marker" },
		func(attr *model.LockAttribute) { attr.Kind = "integer" },
		func(attr *model.LockAttribute) { attr.From.Symbol = ""; attr.From.Constant = "private-marker" },
	} {
		assertMalformedMetadataLock(t, encoded, mutate)
	}
}

func assertMalformedMetadataLock(t *testing.T, encoded []byte, mutate func(*model.LockAttribute)) {
	t.Helper()

	var locked model.Lockfile

	err := json.Unmarshal(encoded, &locked)
	if err != nil {
		t.Fatal(err)
	}

	mutate(&locked.Targets[0].Attributes[0])

	malformed, err := json.Marshal(locked)
	if err != nil {
		t.Fatal(err)
	}

	_, err = lockfile.Parse(malformed)
	if err == nil || strings.Contains(err.Error(), "private-marker") {
		t.Fatalf("malformed metadata lock was accepted or exposed values: %v", err)
	}
}
