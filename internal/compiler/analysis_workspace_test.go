package compiler_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/modfile"
)

func analyzedModuleFixture(t *testing.T) (*model.CodeModel, *os.Root) {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	code := new(model.CodeModel)
	code.ModuleRoot = filepath.Join(root.Name(), "cmd")
	code.Modules = []model.ModuleInfo{
		{Main: true, Dir: root.Name(), Path: "example.com/app", Version: "", Ownership: "", Replace: nil},
	}
	code.EffectiveBuild.GoVersion, code.EffectiveBuild.ModFile = "go1.27.0", filepath.Join(root.Name(), "alternate.mod")

	return code, root
}

func analyzedWorkspaceFixture(t *testing.T) (*model.CodeModel, *os.Root) {
	t.Helper()

	code, root := analyzedModuleFixture(t)
	code.WorkspaceFile, code.EffectiveBuild.Workspace, code.EffectiveBuild.ModFile =
		filepath.Join(root.Name(), "go.work"), true, ""

	err := root.WriteFile("go.work", []byte("go 1.27.0\nuse ./app\nreplace example.com/dep => ./dep\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return code, root
}

func TestWorkspaceForModuleAnalysis(t *testing.T) {
	t.Parallel()

	code, root := analyzedModuleFixture(t)

	request, err := compiler.WorkspaceForAnalysis(code)
	if err != nil {
		t.Fatal(err)
	}

	assertSyntheticWorkspace(t, request.Workspace, root.Name())

	if request.AlternateModFiles[root.Name()] != code.EffectiveBuild.ModFile {
		t.Fatal("alternate manifest mapped to analysis subdirectory")
	}

	again, err := compiler.WorkspaceForAnalysis(code)
	if err != nil || !bytes.Equal(request.Workspace, again.Workspace) {
		t.Fatal("workspace changed between identical requests")
	}

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil || len(entries) != 0 {
		t.Fatal("planning wrote project files", err)
	}
}

func TestWorkspaceForExistingAnalysis(t *testing.T) {
	t.Parallel()

	code, root := analyzedWorkspaceFixture(t)
	sums := []byte("workspace checksums\n")

	err := root.WriteFile("go.work.sum", sums, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	original, err := root.ReadFile("go.work")
	if err != nil {
		t.Fatal(err)
	}

	request, err := compiler.WorkspaceForAnalysis(code)
	if err != nil || !bytes.Equal(request.Workspace, original) ||
		!bytes.Equal(request.WorkspaceSums, sums) || request.OriginalWorkspaceDir != root.Name() {
		t.Fatal("existing workspace was not preserved", err)
	}

	after, err := root.ReadFile("go.work")
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("planning changed the workspace input", err)
	}
}

func TestWorkspaceForAnalysisRejectsInconsistentIdentity(t *testing.T) {
	t.Parallel()

	code, root := analyzedWorkspaceFixture(t)
	for _, change := range []func(*model.CodeModel){
		func(c *model.CodeModel) { c.EffectiveBuild.GoVersion = "invalid" },
		func(c *model.CodeModel) { c.WorkspaceFile = "relative.work" },
		func(c *model.CodeModel) { c.WorkspaceFile = filepath.Join(root.Name(), "missing.work") },
		func(c *model.CodeModel) { c.EffectiveBuild.ModFile = filepath.Join(root.Name(), "alternate.mod") },
		func(c *model.CodeModel) { c.EffectiveBuild.Workspace = false },
	} {
		invalid := *code
		change(&invalid)

		_, err := compiler.WorkspaceForAnalysis(&invalid)
		if err == nil {
			t.Fatal("accepted inconsistent analysis")
		}
	}

	_, err := compiler.WorkspaceForAnalysis(nil)
	if err == nil {
		t.Fatal("accepted missing analysis")
	}
}

func TestWorkspacePlanningMissingInputPreservesCause(t *testing.T) {
	t.Parallel()

	code, root := analyzedWorkspaceFixture(t)

	err := root.Remove("go.work")
	if err != nil {
		t.Fatal(err)
	}

	_, err = compiler.WorkspaceForAnalysis(code)
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "read analyzed workspace manifest") {
		t.Fatal("missing workspace lost its filesystem cause or operation context", err)
	}
}

func TestWorkspacePlanningParserFailurePreservesCause(t *testing.T) {
	t.Parallel()

	code, root := analyzedWorkspaceFixture(t)

	err := root.WriteFile("go.work", []byte("go invalid\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	_, err = compiler.WorkspaceForAnalysis(code)

	var parseErrors modfile.ErrorList
	if !errors.As(err, &parseErrors) || !strings.Contains(err.Error(), "invalid analyzed workspace manifest") {
		t.Fatal("invalid workspace lost its parser cause or operation context", err)
	}
}

func TestWorkspacePlanningInvalidIdentitySharesCause(t *testing.T) {
	t.Parallel()

	_, first := compiler.WorkspaceForAnalysis(nil)

	_, second := compiler.WorkspaceForAnalysis(nil)
	if first == nil || !errors.Is(second, first) ||
		first.Error() != "workspace preparation requires analyzed root and Go version" {
		t.Fatal("invalid analysis lost its stable cause or meaningful message")
	}
}

func assertSyntheticWorkspace(t *testing.T, data []byte, directory string) {
	t.Helper()

	work, err := modfile.ParseWork("go.work", data, nil)
	if err != nil {
		t.Fatal(err)
	}

	if work.Go.Version != "1.27.0" || len(work.Use) != 1 || work.Use[0].Path != directory {
		t.Fatalf("wrong synthetic workspace: %s", data)
	}
}

func TestWorkspacePlanningPreservesLogicalSymlinkIdentity(t *testing.T) {
	t.Parallel()

	code, root := analyzedWorkspaceFixture(t)

	contents, err := root.ReadFile("go.work")
	if err != nil {
		t.Fatal(err)
	}

	moveWorkspaceFixtureToOwnedSymlink(t, root, contents)

	err = root.WriteFile("go.work.sum", []byte("logical checksums"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	request, err := compiler.WorkspaceForAnalysis(code)
	if err != nil || request.OriginalWorkspaceDir != root.Name() || !bytes.Equal(request.Workspace, contents) ||
		string(request.WorkspaceSums) != "logical checksums" {
		t.Fatal("caller-owned symlink changed workspace directory or sidecar identity", err)
	}
}

func TestWorkspacePlanningChecksumFailurePreservesCause(t *testing.T) {
	t.Parallel()

	code, root := analyzedWorkspaceFixture(t)

	err := root.Mkdir("go.work.sum", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	_, err = compiler.WorkspaceForAnalysis(code)

	var pathError *os.PathError
	if !errors.As(err, &pathError) || !strings.Contains(err.Error(), "read analyzed workspace checksums") {
		t.Fatal("unreadable checksums lost their filesystem cause or operation context", err)
	}
}

func moveWorkspaceFixtureToOwnedSymlink(t *testing.T, root *os.Root, contents []byte) {
	t.Helper()

	outside, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = outside.Close() }()

	err = outside.WriteFile("shared.work", contents, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = root.Remove("go.work")
	if err != nil {
		t.Fatal(err)
	}

	err = root.Symlink(filepath.Join(outside.Name(), "shared.work"), "go.work")
	if err != nil {
		t.Fatal(err)
	}
}
