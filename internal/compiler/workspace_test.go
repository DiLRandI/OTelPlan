package compiler_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
)

type workspacePreparationFixture struct {
	root    *os.Root
	request compiler.WorkspaceRequest
	files   map[string]string
}

func newWorkspacePreparationFixture(t *testing.T) workspacePreparationFixture {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	files := map[string]string{
		"app/go.mod":  "module example.com/app\n\ngo 1.25.0\n",
		"app/main.go": "package main\nimport \"fmt\"\nfunc main(){fmt.Print(\"prepared\")}\n",
	}
	writeRelocationFixture(t, root, files)

	var request compiler.WorkspaceRequest

	request.OriginalWorkspaceDir = filepath.Join(root.Name(), "app")
	request.SourceDirs = []string{request.OriginalWorkspaceDir}
	request.Workspace = []byte("go 1.25.0\nuse .\n")
	request.WorkspaceSums = []byte("example.com/unused v1.0.0 h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n")
	request.Runtime = generatedArtifactBundle(t)
	request.Parent = t.TempDir()

	return workspacePreparationFixture{root: root, request: request, files: files}
}

func TestPrepareWorkspace(t *testing.T) {
	t.Parallel()

	fixture := newWorkspacePreparationFixture(t)
	request := fixture.request

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	request.Parent, err = filepath.Rel(cwd, request.Parent)
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := compiler.PrepareWorkspace(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	if !filepath.IsAbs(prepared.WorkspaceFile) {
		t.Fatal("workspace path must be absolute")
	}

	err = compiler.VerifyArtifacts(prepared.Runtime)
	if err != nil {
		t.Fatal(err)
	}

	command := exec.CommandContext(t.Context(), "go", "run", "-mod=readonly", ".")
	command.Dir = prepared.Relocations[request.OriginalWorkspaceDir]
	command.Env = append(os.Environ(), "GOWORK="+prepared.WorkspaceFile, "GOFLAGS=", "GOPROXY=off")

	output, err := command.CombinedOutput()
	if err != nil || string(output) != "prepared" {
		t.Fatalf("prepared build failed: %v\n%s", err, output)
	}

	assertPreparedWorkspaceCopies(t, fixture, prepared)
}

func assertPreparedWorkspaceCopies(t *testing.T, fixture workspacePreparationFixture,
	prepared compiler.PreparedWorkspace) {
	t.Helper()

	root, err := os.OpenRoot(prepared.Dir)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	sums, err := root.ReadFile("go.work.sum")
	if err != nil || !bytes.Equal(sums, fixture.request.WorkspaceSums) {
		t.Fatal("workspace checksums were not preserved", err)
	}

	sourceRoot, err := os.OpenRoot(prepared.Relocations[fixture.request.OriginalWorkspaceDir])
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = sourceRoot.Close() }()

	err = sourceRoot.WriteFile("main.go", []byte("modified"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	assertCopySourceUnchanged(t, fixture.root, fixture.files)

	err = compiler.VerifyArtifacts(fixture.request.Runtime)
	if err != nil {
		t.Fatal("original runtime changed", err)
	}
}

func TestPrepareWorkspaceCleansRelocationFailure(t *testing.T) {
	t.Parallel()

	fixture := newWorkspacePreparationFixture(t)
	request := fixture.request
	request.Workspace = []byte("go 1.25.0\nuse ./missing\n")

	_, err := compiler.PrepareWorkspace(t.Context(), request)
	if err == nil {
		t.Fatal("accepted uncopied workspace module")
	}

	assertAlternatePreparationCleaned(t, request)
	assertCopySourceUnchanged(t, fixture.root, fixture.files)
}

func TestPrepareWorkspaceRejectsRuntimeTampering(t *testing.T) {
	t.Parallel()

	fixture := newWorkspacePreparationFixture(t)

	root, err := os.OpenRoot(fixture.request.Runtime.Dir)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = root.WriteFile("manifest.json", []byte("modified"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	_, err = compiler.PrepareWorkspace(t.Context(), fixture.request)
	if err == nil {
		t.Fatal("accepted modified runtime artifacts")
	}

	assertAlternatePreparationCleaned(t, fixture.request)
	assertCopySourceUnchanged(t, fixture.root, fixture.files)
}

func TestPrepareWorkspaceCancellation(t *testing.T) {
	t.Parallel()

	fixture := newWorkspacePreparationFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := compiler.PrepareWorkspace(ctx, fixture.request)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("workspace preparation lost cancellation", err)
	}

	assertAlternatePreparationCleaned(t, fixture.request)
	assertCopySourceUnchanged(t, fixture.root, fixture.files)
}

func TestWorkspacePreparationValidationSharesCauses(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"relative source", "duplicate sources", "unknown alternate"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fixture := newWorkspacePreparationFixture(t)
			request := fixture.request
			expected := "source module directories must be unique absolute paths"

			switch name {
			case "relative source":
				request.SourceDirs = []string{"relative"}
			case "duplicate sources":
				request.SourceDirs = append(request.SourceDirs, request.SourceDirs...)
			case "unknown alternate":
				request.AlternateModFiles = map[string]string{
					t.TempDir(): filepath.Join(fixture.root.Name(), "build.mod"),
				}
				expected = "alternate module file requires a known module and an absolute .mod path"
			}

			_, first := compiler.PrepareWorkspace(t.Context(), request)

			_, second := compiler.PrepareWorkspace(t.Context(), request)
			if first == nil || !errors.Is(second, first) || first.Error() != expected {
				t.Fatal("invalid workspace selection lost its stable cause or meaningful message", first, second)
			}

			assertAlternatePreparationCleaned(t, request)
			assertCopySourceUnchanged(t, fixture.root, fixture.files)
		})
	}
}

func TestPrepareWorkspaceDoesNotReorderCallerSources(t *testing.T) {
	t.Parallel()

	fixture := newWorkspacePreparationFixture(t)
	writeRelocationFixture(t, fixture.root, map[string]string{
		"z/go.mod": "module example.com/z\n\ngo 1.25.0\n",
	})
	request := fixture.request
	request.SourceDirs = []string{filepath.Join(fixture.root.Name(), "z"), request.OriginalWorkspaceDir}
	before := append([]string(nil), request.SourceDirs...)

	_, err := compiler.PrepareWorkspace(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(before, request.SourceDirs) {
		t.Fatal("preparation reordered caller-owned source directories")
	}
}

func TestPrepareWorkspaceMissingCopiedManifestPreservesCause(t *testing.T) {
	t.Parallel()

	fixture := newWorkspacePreparationFixture(t)

	err := fixture.root.Remove(filepath.Join("app", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = compiler.PrepareWorkspace(t.Context(), fixture.request)
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "read copied module manifest") {
		t.Fatal("missing copied manifest lost its cause or operation", err)
	}

	assertAlternatePreparationCleaned(t, fixture.request)
}
