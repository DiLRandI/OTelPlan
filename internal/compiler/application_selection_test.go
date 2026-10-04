package compiler

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type selectionFixture struct {
	workspace   PreparedWorkspace
	application string
	root        *os.Root
	files       map[string]string
}

func applicationSelectionFixture(t *testing.T) selectionFixture {
	t.Helper()

	directory := t.TempDir()
	app, generated := filepath.Join(directory, "app"), filepath.Join(directory, "runtime")
	files := map[string]string{
		"app/go.mod": "module example.com/app\n\ngo 1.27.0\n" +
			"require example.com/dep v1.0.0\nreplace example.com/dep => ../dep\n",
		"runtime/go.mod": "module example.com/runtime\n\ngo 1.27.0\nrequire example.com/dep v1.1.0\n",
		"dep/go.mod":     "module example.com/dep\n\ngo 1.27.0\n",
		"go.work":        fmt.Sprintf("go 1.27.0\nuse (\n%q\n%q\n)\n", app, generated),
	}

	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	for path, data := range files {
		err := root.MkdirAll(filepath.Dir(path), 0o700)
		if err != nil {
			t.Fatal(err)
		}

		err = root.WriteFile(path, []byte(data), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	workspace := new(PreparedWorkspace)
	workspace.Dir, workspace.WorkspaceFile = directory, filepath.Join(directory, "go.work")
	workspace.Runtime.Dir = generated

	return selectionFixture{workspace: *workspace, application: app, root: root, files: files}
}

func selectionEnvironment() []string {
	return append(os.Environ(), "GOFLAGS=", "GOPROXY=off", "GONOPROXY=none", "GOSUMDB=off")
}

func assertSelectionFixtureUnchanged(t *testing.T, fixture selectionFixture) {
	t.Helper()

	for path, expected := range fixture.files {
		data, err := fixture.root.ReadFile(path)
		if err != nil || string(data) != expected {
			t.Fatal("selection changed workspace files", err)
		}
	}

	entries, err := fs.ReadDir(fixture.root.FS(), ".")
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "application-") {
			t.Fatal("selection left temporary workspace")
		}
	}
}

func TestApplicationSelectionExcludesRuntimeUpgrade(t *testing.T) {
	t.Parallel()

	fixture := applicationSelectionFixture(t)
	env := selectionEnvironment()

	baseline, err := applicationModuleSelection(t.Context(), fixture.workspace, fixture.application, env)
	if err != nil {
		t.Fatal(err)
	}

	selected, err := ReadModuleSelection(t.Context(), fixture.application,
		append(env, "GOWORK="+fixture.workspace.WorkspaceFile))
	if err != nil {
		t.Fatal(err)
	}

	err = CheckModuleSelection(baseline, selected, nil)
	if err == nil {
		t.Fatal("runtime silently upgraded an application dependency")
	}

	found := false

	for _, module := range baseline {
		if module.Path == "example.com/dep" && module.Version == "v1.0.0" {
			found = true
		}
	}

	if !found {
		t.Fatal("baseline omitted application dependency")
	}

	assertSelectionFixtureUnchanged(t, fixture)
}

func TestApplicationSelectionMissingRuntimeSharesCause(t *testing.T) {
	t.Parallel()

	fixture := applicationSelectionFixture(t)

	err := fixture.root.WriteFile("go.work", []byte(fmt.Sprintf("go 1.27\nuse %q\n", fixture.application)), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	_, first := applicationModuleSelection(t.Context(), fixture.workspace, fixture.application, selectionEnvironment())

	_, second := applicationModuleSelection(t.Context(), fixture.workspace, fixture.application, selectionEnvironment())
	if first == nil || !errors.Is(second, first) || first.Error() != "prepared workspace has no generated runtime" {
		t.Fatal("missing runtime lost its stable cause or meaningful error")
	}
}

func TestApplicationSelectionChecksumFailureCleansUp(t *testing.T) {
	t.Parallel()

	fixture := applicationSelectionFixture(t)

	err := fixture.root.Mkdir("go.work.sum", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	modules, err := applicationModuleSelection(t.Context(), fixture.workspace, fixture.application, selectionEnvironment())
	if err == nil || modules != nil || !strings.Contains(err.Error(), "read prepared workspace checksums") {
		t.Fatal("checksum failure was silently ignored or lost operation context", err)
	}

	assertSelectionFixtureUnchanged(t, fixture)
}

func TestApplicationSelectionCancellationLeavesNoInputs(t *testing.T) {
	t.Parallel()

	fixture := applicationSelectionFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	modules, err := applicationModuleSelection(ctx, fixture.workspace, fixture.application, selectionEnvironment())
	if modules != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("application baseline cancellation was not preserved", err)
	}

	assertSelectionFixtureUnchanged(t, fixture)
}

func TestSelectionWriterDoesNotReplaceExistingInput(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = root.WriteFile("owned.work", []byte("original input"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = writeSelectionFile(root, "owned.work", []byte("replacement"))
	if !errors.Is(err, fs.ErrExist) {
		t.Fatal("temporary workspace creation did not reject an existing input", err)
	}

	contents, err := root.ReadFile("owned.work")
	if err != nil || string(contents) != "original input" {
		t.Fatal("failed workspace creation replaced or removed caller-owned input", err)
	}
}

func TestApplicationSelectionPreservesExistingChecksums(t *testing.T) {
	t.Parallel()

	fixture := applicationSelectionFixture(t)
	fixture.files["go.work.sum"] = ""

	err := fixture.root.WriteFile("go.work.sum", nil, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	_, err = applicationModuleSelection(t.Context(), fixture.workspace, fixture.application, selectionEnvironment())
	if err != nil {
		t.Fatal(err)
	}

	assertSelectionFixtureUnchanged(t, fixture)
}

func TestSelectionChecksumCopyDoesNotReplaceExistingInput(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = root.WriteFile("go.work.sum", []byte("source checksums"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = root.WriteFile("owned.work.sum", []byte("original checksums"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = copySelectionChecksums(root, "go.work", "owned.work")
	if !errors.Is(err, fs.ErrExist) {
		t.Fatal("checksum copy did not reject an existing input", err)
	}

	contents, err := root.ReadFile("owned.work.sum")
	if err != nil || string(contents) != "original checksums" {
		t.Fatal("checksum copy replaced or removed caller-owned input", err)
	}
}
