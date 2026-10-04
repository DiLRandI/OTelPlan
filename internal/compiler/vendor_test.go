package compiler

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type vendorWorkspaceFixture struct {
	root      *os.Root
	workspace PreparedWorkspace
	env       []string
	original  []byte
}

func newVendorWorkspaceFixture(t *testing.T) vendorWorkspaceFixture {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	files := map[string]string{
		"app/go.mod": "module example.com/app\n\ngo 1.27\n" +
			"require example.com/dep v0.0.0\nreplace example.com/dep => ../dep\n",
		"app/app.go": "package app\nimport \"example.com/dep\"\nfunc Name() string { return dep.Name() }\n",
		"dep/go.mod": "module example.com/dep\n\ngo 1.27\n",
		"dep/dep.go": "package dep\nfunc Name() string { return \"original\" }\n",
		"runtime/go.mod": "module example.com/runtime\n\ngo 1.27\n" +
			"require example.com/dep v0.0.0\nreplace example.com/dep => ../dep\n",
		"runtime/run.go": "package runtime\nimport \"example.com/dep\"\nfunc Name() string { return dep.Name() }\n",
		"go.work":        "go 1.27\nuse (\n./app\n./runtime\n)\n",
	}
	for name, content := range files {
		filename := filepath.FromSlash(name)

		err := root.MkdirAll(filepath.Dir(filename), 0o700)
		if err != nil {
			t.Fatal(err)
		}

		err = root.WriteFile(filename, []byte(content), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	prepareVendorFixtureModules(t, filepath.Join(root.Name(), "app"))

	before, err := root.ReadFile(filepath.FromSlash("app/vendor/example.com/dep/dep.go"))
	if err != nil {
		t.Fatal(err)
	}

	var workspace PreparedWorkspace

	workspace.Dir, workspace.WorkspaceFile = root.Name(), filepath.Join(root.Name(), "go.work")

	env := append(os.Environ(), "GOFLAGS=", "GOPROXY=off")

	return vendorWorkspaceFixture{root: root, workspace: workspace, env: env, original: before}
}

func prepareVendorFixtureModules(t *testing.T, dir string) {
	t.Helper()

	command := exec.CommandContext(t.Context(), "go", "mod", "vendor")
	command.Dir = dir

	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOPROXY=off")

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare fixture vendor tree: %v\n%s", err, output)
	}
}

func listVendorFixture(t *testing.T, fixture vendorWorkspaceFixture) []byte {
	t.Helper()

	command := exec.CommandContext(t.Context(), "go", "list", "-mod=vendor", "./...")
	command.Dir = filepath.Join(fixture.root.Name(), "app")

	command.Env = append(append([]string(nil), fixture.env...), "GOWORK="+fixture.workspace.WorkspaceFile)

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("list isolated vendor packages: %v\n%s", err, output)
	}

	return output
}

func materializeVendorFixture(t *testing.T, fixture vendorWorkspaceFixture) {
	t.Helper()

	original := filepath.Join(fixture.root.Name(), "app", "vendor")

	err := materializeVendorWorkspace(t.Context(), fixture.workspace, original, fixture.env)
	if err != nil {
		t.Fatal(err)
	}
}

func assertVendorFixtureUnchanged(t *testing.T, fixture vendorWorkspaceFixture) {
	t.Helper()

	after, err := fixture.root.ReadFile(filepath.FromSlash("app/vendor/example.com/dep/dep.go"))
	if err != nil || !bytes.Equal(after, fixture.original) {
		t.Fatal("materialization changed the analyzed vendor tree", err)
	}
}

func TestMaterializeVendorWorkspace(t *testing.T) {
	t.Parallel()

	fixture := newVendorWorkspaceFixture(t)
	materializeVendorFixture(t, fixture)

	output := listVendorFixture(t, fixture)
	if !strings.Contains(string(output), "example.com/app") {
		t.Fatalf("isolated vendor mode failed: %s", output)
	}

	assertVendorFixtureUnchanged(t, fixture)
}

func TestMaterializeVendorRejectsAddedPackageFile(t *testing.T) {
	t.Parallel()

	fixture := newVendorWorkspaceFixture(t)
	materializeVendorFixture(t, fixture)

	err := fixture.root.WriteFile(filepath.Join("dep", "extra.go"), []byte("package dep\nfunc Extra() {}\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	original := filepath.Join(fixture.root.Name(), "app", "vendor")

	err = materializeVendorWorkspace(t.Context(), fixture.workspace, original, fixture.env)
	if err == nil || !strings.Contains(err.Error(), "was added") {
		t.Fatalf("accepted an added file in an analyzed vendored package: %v", err)
	}

	assertVendorFixtureUnchanged(t, fixture)

	err = fixture.root.Remove(filepath.Join("dep", "extra.go"))
	if err != nil {
		t.Fatal(err)
	}

	materializeVendorFixture(t, fixture)
	assertVendorFixtureUnchanged(t, fixture)
}

func TestMaterializeVendorCopiesMultiModuleWorkspace(t *testing.T) {
	t.Parallel()

	fixture := newVendorWorkspaceFixture(t)
	materializeVendorFixture(t, fixture)

	copied, err := CopySourceTree(t.Context(), fixture.root.Name(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	var workspace PreparedWorkspace

	workspace.Dir, workspace.WorkspaceFile = copied, filepath.Join(copied, "go.work")
	original := filepath.Join(fixture.root.Name(), "vendor")

	err = materializeVendorWorkspace(t.Context(), workspace, original, fixture.env)
	if err != nil {
		t.Fatalf("prepare vendored multi-module workspace: %v", err)
	}

	assertVendorFixtureUnchanged(t, fixture)
}

func TestMaterializeVendorRejectsChangedContent(t *testing.T) {
	t.Parallel()

	fixture := newVendorWorkspaceFixture(t)
	original := filepath.Join(fixture.root.Name(), "app", "vendor")
	patched := []byte("package dep\nfunc Name() string { return \"patched\" }\n")

	err := fixture.root.WriteFile(filepath.FromSlash("app/vendor/example.com/dep/dep.go"), patched, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = materializeVendorWorkspace(t.Context(), fixture.workspace, original, fixture.env)
	if err == nil || !strings.Contains(err.Error(), "vendor content differs") {
		t.Fatalf("accepted vendor source differing from the isolated build: %v", err)
	}

	fixture.original = patched
	assertVendorFixtureUnchanged(t, fixture)
}

func TestMaterializeVendorPreservesCancellation(t *testing.T) {
	t.Parallel()

	fixture := newVendorWorkspaceFixture(t)
	original := filepath.Join(fixture.root.Name(), "app", "vendor")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := materializeVendorWorkspace(ctx, fixture.workspace, original, fixture.env)
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "materialize isolated vendor workspace") {
		t.Fatalf("vendor preparation ignored cancellation or lost operation context: %v", err)
	}

	assertVendorFixtureUnchanged(t, fixture)
}

func TestVendorNonregularEntriesShareCause(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = root.Mkdir("directory", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	_, first := vendorFileDigest(root, "directory")

	_, second := vendorFileDigest(root, "directory")
	if first == nil || !errors.Is(second, first) || first.Error() != "vendor entry is not a regular file" {
		t.Fatal("nonregular vendor entries lost their stable cause or meaningful message", first, second)
	}
}

func TestVendorChangedFilesShareCause(t *testing.T) {
	t.Parallel()

	original, generated := t.TempDir(), t.TempDir()
	for dir, contents := range map[string]string{original: "original", generated: "changed"} {
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}

		err = root.WriteFile("dependency.go", []byte(contents), 0o600)
		_ = root.Close()

		if err != nil {
			t.Fatal(err)
		}
	}

	first := compareVendorFiles(original, generated)

	second := compareVendorFiles(original, generated)
	cause := first

	for errors.Unwrap(cause) != nil {
		cause = errors.Unwrap(cause)
	}

	if cause == nil || !errors.Is(second, cause) || !strings.Contains(first.Error(), "dependency.go changed") {
		t.Fatal("changed vendor content lost its shared cause or filename", first, second)
	}
}

func TestVendorMissingFilePreservesCause(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = root.WriteFile("dependency.go", []byte("original"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = compareVendorFiles(root.Name(), t.TempDir())
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "read isolated vendor file dependency.go") {
		t.Fatal("missing vendor file lost its cause or operation", err)
	}
}

func TestVendorManifestReadPreservesCause(t *testing.T) {
	t.Parallel()

	var workspace PreparedWorkspace

	workspace.Dir = t.TempDir()
	workspace.WorkspaceFile = filepath.Join(workspace.Dir, "go.work")
	err := materializeVendorWorkspace(t.Context(), workspace, t.TempDir(), nil)

	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "read analyzed vendor manifest") {
		t.Fatal("missing vendor manifest lost its cause or operation", err)
	}
}
