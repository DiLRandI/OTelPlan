package compiler

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/modfile"
)

func TestWorkspaceForAnalysis(t *testing.T) {
	root := t.TempDir()
	code := &model.CodeModel{ModuleRoot: filepath.Join(root, "cmd"), Modules: []model.ModuleInfo{{Main: true, Dir: root}}, EffectiveBuild: model.BuildEnvironment{GoVersion: "go1.27.0", ModFile: filepath.Join(root, "alternate.mod")}}
	request, err := WorkspaceForAnalysis(code)
	if err != nil {
		t.Fatal(err)
	}
	work, err := modfile.ParseWork("go.work", request.Workspace, nil)
	if err != nil || work.Go.Version != "1.27.0" || len(work.Use) != 1 || work.Use[0].Path != root {
		t.Fatalf("wrong synthetic workspace: %s %v", request.Workspace, err)
	}
	if request.AlternateModFiles[root] != code.EffectiveBuild.ModFile {
		t.Fatal("alternate manifest mapped to analysis subdirectory")
	}
	again, err := WorkspaceForAnalysis(code)
	if err != nil || !bytes.Equal(request.Workspace, again.Workspace) {
		t.Fatal("workspace changed between identical requests")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("planning wrote project files")
	}
	code.WorkspaceFile = filepath.Join(root, "go.work")
	code.EffectiveBuild.Workspace = true
	code.EffectiveBuild.ModFile = ""
	original := []byte("go 1.27.0\nuse ./app\nreplace example.com/dep => ./dep\n")
	sums := []byte("workspace checksums\n")
	if err := os.WriteFile(code.WorkspaceFile, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(code.WorkspaceFile+".sum", sums, 0o600); err != nil {
		t.Fatal(err)
	}
	request, err = WorkspaceForAnalysis(code)
	if err != nil || !bytes.Equal(request.Workspace, original) || !bytes.Equal(request.WorkspaceSums, sums) || request.OriginalWorkspaceDir != root {
		t.Fatal("existing workspace was not preserved")
	}
	for _, change := range []func(*model.CodeModel){
		func(c *model.CodeModel) { c.EffectiveBuild.GoVersion = "invalid" },
		func(c *model.CodeModel) { c.WorkspaceFile = "relative.work" },
		func(c *model.CodeModel) { c.WorkspaceFile = filepath.Join(root, "missing.work") },
		func(c *model.CodeModel) { c.EffectiveBuild.ModFile = filepath.Join(root, "alternate.mod") },
		func(c *model.CodeModel) { c.EffectiveBuild.Workspace = false },
	} {
		invalid := *code
		change(&invalid)
		if _, err := WorkspaceForAnalysis(&invalid); err == nil {
			t.Fatal("accepted inconsistent analysis")
		}
	}
	if _, err := WorkspaceForAnalysis(nil); err == nil {
		t.Fatal("accepted missing analysis")
	}
}
