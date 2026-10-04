package compiler_test

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"golang.org/x/mod/modfile"
)

type alternateModuleFixture struct {
	root    *os.Root
	request compiler.WorkspaceRequest
	files   map[string]string
}

func newAlternateModuleFixture(t *testing.T) alternateModuleFixture {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	files := map[string]string{
		"app/go.mod":            "module example.com/original\n\ngo 1.25.0\n",
		"app/go.sum":            "original checksums\n",
		"app/dependency/go.mod": "module example.com/dependency\n\ngo 1.25.0\n",
		"alternate/build.mod": "module example.com/alternate\n\ngo 1.25.0\n" +
			"replace example.com/dependency => ./dependency\n",
	}
	writeRelocationFixture(t, root, files)

	runtimeFiles := []otelc.GeneratedFile{{
		Path: "go.mod", Data: []byte("module example.com/runtime\n\ngo 1.25.0\n"),
	}}

	runtime, err := compiler.StageArtifacts(t.TempDir(), runtimeFiles)
	if err != nil {
		t.Fatal(err)
	}

	var request compiler.WorkspaceRequest

	request.OriginalWorkspaceDir = filepath.Join(root.Name(), "app")
	request.Workspace = []byte("go 1.25.0\nuse .\n")
	request.Runtime = runtime
	request.Parent = t.TempDir()
	request.AlternateModFiles = map[string]string{
		request.OriginalWorkspaceDir: filepath.Join(root.Name(), "alternate", "build.mod"),
	}

	return alternateModuleFixture{root: root, request: request, files: files}
}

func TestPrepareWorkspaceAlternateModfile(t *testing.T) {
	t.Parallel()

	for _, sums := range []string{"", "alternate checksums\n"} {
		t.Run(sums, func(t *testing.T) {
			t.Parallel()

			fixture := newAlternateModuleFixture(t)
			if sums != "" {
				writeRelocationFixture(t, fixture.root, map[string]string{"alternate/build.sum": sums})
				fixture.files["alternate/build.sum"] = sums
			}

			prepared, err := compiler.PrepareWorkspace(t.Context(), fixture.request)
			if err != nil {
				t.Fatal(err)
			}

			assertAlternateModuleCopy(t, fixture.request.OriginalWorkspaceDir, prepared, sums)
			assertCopySourceUnchanged(t, fixture.root, fixture.files)
		})
	}
}

func assertAlternateModuleCopy(t *testing.T, source string, prepared compiler.PreparedWorkspace, sums string) {
	t.Helper()

	root, err := os.OpenRoot(prepared.Relocations[source])
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	manifest, err := root.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := modfile.Parse("go.mod", manifest, nil)

	dependency := prepared.Relocations[filepath.Join(source, "dependency")]
	if err != nil || len(parsed.Replace) != 1 || parsed.Replace[0].New.Path != dependency {
		t.Fatal("alternate replacement did not resolve relative to original module", err)
	}

	assertAlternateModuleIdentity(t, root.Name())

	copiedSums, err := root.ReadFile("go.sum")
	if sums == "" {
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("original checksums survived alternate selection", err)
		}
	} else if err != nil || string(copiedSums) != sums {
		t.Fatal("alternate checksums not copied", err)
	}
}

func assertAlternateModuleIdentity(t *testing.T, dir string) {
	t.Helper()

	command := exec.CommandContext(t.Context(), "go", "list", "-m")
	command.Dir = dir

	command.Env = append(os.Environ(), "GOFLAGS=", "GOWORK=off", "GOPROXY=off")

	output, err := command.CombinedOutput()
	if err != nil || string(output) != "example.com/alternate\n" {
		t.Fatalf("alternate manifest not used: %v %s", err, output)
	}
}

func assertAlternatePreparationCleaned(t *testing.T, request compiler.WorkspaceRequest) {
	t.Helper()

	entries, err := os.ReadDir(request.Parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("invalid selection left output", err)
	}
}

func TestPrepareWorkspaceRejectsInvalidAlternateSelection(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"unknown module", "relative", "missing", "malformed", "extension"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newAlternateModuleFixture(t)
			request := fixture.request

			writeRelocationFixture(t, fixture.root, map[string]string{"alternate/malformed.mod": "not a module manifest\n"})

			selection := map[string]map[string]string{
				"unknown module": {t.TempDir(): filepath.Join(fixture.root.Name(), "alternate", "build.mod")},
				"relative":       {request.OriginalWorkspaceDir: "relative.mod"},
				"missing":        {request.OriginalWorkspaceDir: filepath.Join(fixture.root.Name(), "alternate", "missing.mod")},
				"malformed":      {request.OriginalWorkspaceDir: filepath.Join(fixture.root.Name(), "alternate", "malformed.mod")},
				"extension":      {request.OriginalWorkspaceDir: filepath.Join(fixture.root.Name(), "alternate", "wrong.txt")},
			}
			request.AlternateModFiles = selection[name]

			_, err := compiler.PrepareWorkspace(t.Context(), request)
			if err == nil {
				t.Fatal("accepted invalid alternate module selection")
			}

			assertAlternatePreparationCleaned(t, request)
			assertCopySourceUnchanged(t, fixture.root, fixture.files)
		})
	}
}

func TestAlternateManifestInstallationPreservesMissingCause(t *testing.T) {
	t.Parallel()

	fixture := newAlternateModuleFixture(t)
	request := fixture.request
	request.SourceDirs = []string{request.OriginalWorkspaceDir}
	missing := filepath.Join(fixture.root.Name(), "alternate", "missing.mod")
	request.AlternateModFiles[request.OriginalWorkspaceDir] = missing

	_, err := compiler.PrepareWorkspace(t.Context(), request)
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "read alternate module manifest") {
		t.Fatal("missing alternate manifest lost its cause or operation", err)
	}

	assertAlternatePreparationCleaned(t, request)
	assertCopySourceUnchanged(t, fixture.root, fixture.files)
}

func TestAlternateChecksumInstallationPreservesFilesystemCause(t *testing.T) {
	t.Parallel()

	fixture := newAlternateModuleFixture(t)

	err := fixture.root.Mkdir(filepath.Join("alternate", "build.sum"), 0o700)
	if err != nil {
		t.Fatal(err)
	}

	_, err = compiler.PrepareWorkspace(t.Context(), fixture.request)

	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) || !strings.Contains(err.Error(), "read alternate module checksums") {
		t.Fatal("unreadable alternate checksums lost their cause or operation", err)
	}

	assertAlternatePreparationCleaned(t, fixture.request)
	assertCopySourceUnchanged(t, fixture.root, fixture.files)
}

func TestAlternateManifestSymlinkUsesLogicalChecksums(t *testing.T) {
	t.Parallel()

	fixture := newAlternateModuleFixture(t)
	sums := "selected checksums\n"
	writeRelocationFixture(t, fixture.root, map[string]string{
		"alternate/build.sum":   sums,
		"physical/selected.mod": fixture.files["alternate/build.mod"],
		"physical/selected.sum": "unselected checksums\n",
	})

	err := fixture.root.Remove(filepath.Join("alternate", "build.mod"))
	if err != nil {
		t.Fatal(err)
	}

	err = fixture.root.Symlink(filepath.Join("..", "physical", "selected.mod"), filepath.Join("alternate", "build.mod"))
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := compiler.PrepareWorkspace(t.Context(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}

	assertAlternateModuleCopy(t, fixture.request.OriginalWorkspaceDir, prepared, sums)
	assertCopySourceUnchanged(t, fixture.root, fixture.files)
}
