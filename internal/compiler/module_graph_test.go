package compiler

import (
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestCheckModuleSelection(t *testing.T) {
	originalDir := t.TempDir()
	isolatedDir := t.TempDir()
	original := []model.ModuleInfo{{Path: "example.com/app", Main: true, Dir: originalDir}, {Path: "example.com/dep", Version: "v1.0.0"}}
	selected := append([]model.ModuleInfo(nil), original...)
	selected[0].Dir = isolatedDir
	selected = append(selected, model.ModuleInfo{Path: "example.com/runtime", Version: "v1.0.0"})

	mapping := map[string]string{originalDir: isolatedDir}

	err := CheckModuleSelection(original, selected, mapping)
	if err != nil {
		t.Fatal(err)
	}

	err = CheckModuleSelection(original, selected, nil)
	if err == nil {
		t.Fatal("accepted unverified relocation")
	}

	for _, version := range []string{"v0.9.0", "v1.1.0"} {
		selected[1].Version = version

		err := CheckModuleSelection(original, selected, mapping)
		if err == nil {
			t.Fatal("accepted dependency version change")
		}
	}

	err = CheckModuleSelection(original, selected[:1], mapping)
	if err == nil {
		t.Fatal("accepted missing dependency")
	}
}

func TestCheckModuleReplacements(t *testing.T) {
	originalDir := t.TempDir()
	isolatedDir := t.TempDir()
	original := []model.ModuleInfo{{Path: "example.com/dep", Version: "v1.0.0", Replace: &model.ModuleReplacement{Path: "../local", Dir: originalDir}}}

	selected := []model.ModuleInfo{{Path: "example.com/dep", Version: "v1.0.0", Replace: &model.ModuleReplacement{Path: isolatedDir, Dir: isolatedDir}}}

	err := CheckModuleSelection(original, selected, map[string]string{originalDir: isolatedDir})
	if err != nil {
		t.Fatal(err)
	}

	selected[0].Replace.Dir = filepath.Join(isolatedDir, "other")

	err = CheckModuleSelection(original, selected, map[string]string{originalDir: isolatedDir})
	if err == nil {
		t.Fatal("accepted changed local replacement")
	}

	original[0].Replace = &model.ModuleReplacement{Path: "example.com/fork", Version: "v1.2.0"}

	selected[0].Replace = &model.ModuleReplacement{Path: "example.com/fork", Version: "v1.2.1"}

	err = CheckModuleSelection(original, selected, nil)
	if err == nil {
		t.Fatal("accepted replacement upgrade")
	}

	selected[0].Replace = nil

	err = CheckModuleSelection(original, selected, nil)
	if err == nil {
		t.Fatal("accepted removed replacement")
	}
}

func TestCheckModuleSelectionRejectsAmbiguousInventory(t *testing.T) {
	module := model.ModuleInfo{Path: "example.com/dep", Version: "v1.0.0"}
	for _, modules := range [][]model.ModuleInfo{{{Path: ""}}, {module, module}} {
		err := CheckModuleSelection(modules, []model.ModuleInfo{module}, nil)
		if err == nil {
			t.Fatal("accepted ambiguous original graph")
		}

		err = CheckModuleSelection([]model.ModuleInfo{module}, modules, nil)
		if err == nil {
			t.Fatal("accepted ambiguous selected graph")
		}
	}
}
