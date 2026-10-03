package compiler_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestCheckModuleSelection(t *testing.T) {
	t.Parallel()

	originalDir := t.TempDir()
	isolatedDir := t.TempDir()
	app := moduleFixture("example.com/app", "")
	app.Main, app.Dir = true, originalDir
	original := []model.ModuleInfo{app, moduleFixture("example.com/dep", "v1.0.0")}
	selected := append([]model.ModuleInfo(nil), original...)
	selected[0].Dir = isolatedDir
	selected = append(selected, moduleFixture("example.com/runtime", "v1.0.0"))

	mapping := map[string]string{originalDir: isolatedDir}

	err := compiler.CheckModuleSelection(original, selected, mapping)
	if err != nil {
		t.Fatal(err)
	}

	err = compiler.CheckModuleSelection(original, selected, nil)
	if err == nil {
		t.Fatal("accepted unverified relocation")
	}

	for _, version := range []string{"v0.9.0", "v1.1.0"} {
		selected[1].Version = version

		err := compiler.CheckModuleSelection(original, selected, mapping)
		if err == nil {
			t.Fatal("accepted dependency version change")
		}
	}

	err = compiler.CheckModuleSelection(original, selected[:1], mapping)
	if err == nil {
		t.Fatal("accepted missing dependency")
	}
}

func TestCheckModuleReplacements(t *testing.T) {
	t.Parallel()

	originalDir := t.TempDir()
	isolatedDir := t.TempDir()
	originalModule := moduleFixture("example.com/dep", "v1.0.0")
	originalModule.Replace = &model.ModuleReplacement{Path: "../local", Dir: originalDir, Version: ""}
	original := []model.ModuleInfo{originalModule}

	selectedModule := moduleFixture("example.com/dep", "v1.0.0")
	selectedModule.Replace = &model.ModuleReplacement{Path: isolatedDir, Dir: isolatedDir, Version: ""}
	selected := []model.ModuleInfo{selectedModule}

	err := compiler.CheckModuleSelection(original, selected, map[string]string{originalDir: isolatedDir})
	if err != nil {
		t.Fatal(err)
	}

	selected[0].Replace.Dir = filepath.Join(isolatedDir, "other")

	err = compiler.CheckModuleSelection(original, selected, map[string]string{originalDir: isolatedDir})
	if err == nil {
		t.Fatal("accepted changed local replacement")
	}

	original[0].Replace = &model.ModuleReplacement{Path: "example.com/fork", Version: "v1.2.0", Dir: ""}

	selected[0].Replace = &model.ModuleReplacement{Path: "example.com/fork", Version: "v1.2.1", Dir: ""}

	err = compiler.CheckModuleSelection(original, selected, nil)
	if err == nil {
		t.Fatal("accepted replacement upgrade")
	}

	selected[0].Replace = nil

	err = compiler.CheckModuleSelection(original, selected, nil)
	if err == nil {
		t.Fatal("accepted removed replacement")
	}
}

func TestCheckModuleSelectionRejectsAmbiguousInventory(t *testing.T) {
	t.Parallel()

	module := moduleFixture("example.com/dep", "v1.0.0")
	for _, modules := range [][]model.ModuleInfo{{moduleFixture("", "")}, {module, module}} {
		err := compiler.CheckModuleSelection(modules, []model.ModuleInfo{module}, nil)
		if err == nil {
			t.Fatal("accepted ambiguous original graph")
		}

		err = compiler.CheckModuleSelection([]model.ModuleInfo{module}, modules, nil)
		if err == nil {
			t.Fatal("accepted ambiguous selected graph")
		}
	}
}

func moduleFixture(path, version string) model.ModuleInfo {
	return model.ModuleInfo{Path: path, Version: version, Main: false, Dir: "", Ownership: "", Replace: nil}
}

func TestModuleInventoryErrorsShareIdentity(t *testing.T) {
	t.Parallel()

	empty := []model.ModuleInfo{moduleFixture("", "")}
	valid := []model.ModuleInfo{moduleFixture("example.com/dep", "v1.0.0")}
	first := compiler.CheckModuleSelection(empty, valid, nil)

	second := compiler.CheckModuleSelection(valid, empty, nil)
	if first == nil || !errors.Is(second, first) {
		t.Fatal("invalid inventory errors do not preserve a shared cause")
	}
}

func TestModuleSelectionKeepsCallerState(t *testing.T) {
	t.Parallel()

	original := []model.ModuleInfo{moduleFixture("example.com/z", "v1.0.0"), moduleFixture("example.com/a", "v1.0.0")}
	selected := append([]model.ModuleInfo(nil), original...)
	before := append([]model.ModuleInfo(nil), original...)
	relocations := map[string]string{"original": "isolated"}

	err := compiler.CheckModuleSelection(original, selected, relocations)
	if err != nil || !reflect.DeepEqual(original, before) || !reflect.DeepEqual(selected, before) ||
		relocations["original"] != "isolated" {
		t.Fatal("module selection validation changed caller state", err)
	}
}

func TestModuleReplacementDriftMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		change  func(*model.ModuleInfo)
		message string
	}{
		{name: "version", change: func(m *model.ModuleInfo) { m.Version = "v1.1.0" },
			message: "build module selection changed example.com/dep"},
		{name: "main flag", change: func(m *model.ModuleInfo) { m.Main = true },
			message: "build module selection changed example.com/dep"},
		{name: "replacement path", change: func(m *model.ModuleInfo) { m.Replace.Path = "example.com/other" },
			message: "build module replacement changed example.com/dep"},
		{name: "replacement version", change: func(m *model.ModuleInfo) { m.Replace.Version = "v1.2.1" },
			message: "build module replacement changed example.com/dep"},
		{name: "removed replacement", change: func(m *model.ModuleInfo) { m.Replace = nil },
			message: "build module replacement changed example.com/dep"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			wanted := moduleFixture("example.com/dep", "v1.0.0")
			wanted.Replace = &model.ModuleReplacement{Path: "example.com/fork", Version: "v1.2.0", Dir: ""}
			actual := wanted
			replacement := *wanted.Replace
			actual.Replace = &replacement
			testCase.change(&actual)

			err := compiler.CheckModuleSelection([]model.ModuleInfo{wanted}, []model.ModuleInfo{actual}, nil)
			if err == nil || err.Error() != testCase.message {
				t.Fatalf("error = %v; want %s", err, testCase.message)
			}
		})
	}
}

func TestModuleSelectionDeterministicFailure(t *testing.T) {
	t.Parallel()

	original := []model.ModuleInfo{moduleFixture("example.com/z", "v1.0.0"), moduleFixture("example.com/a", "v1.0.0")}

	err := compiler.CheckModuleSelection(original, nil, nil)
	if err == nil || err.Error() != "build module selection removed example.com/a" {
		t.Fatal("module error ordering is not deterministic", err)
	}
}
