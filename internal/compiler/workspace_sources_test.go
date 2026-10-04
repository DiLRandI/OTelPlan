package compiler_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
)

type workspaceSourceFixture struct {
	root      *os.Root
	workspace []byte
	alternate map[string]string
	files     map[string]string
}

func newWorkspaceSourceFixture(t *testing.T) workspaceSourceFixture {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	files := map[string]string{
		"app/go.mod": "module example.com/app\nreplace example.com/ignored => ../missing\n",
		"alternate/build.mod": "module example.com/app\nreplace example.com/local => ../local\n" +
			"replace example.com/versioned => example.com/remote v1.0.0\n",
		"local/go.mod":    "module example.com/local\nreplace example.com/app => ../app\n",
		"override/go.mod": "module example.com/override\n",
	}
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

	return workspaceSourceFixture{
		root: root, workspace: []byte("go 1.25.0\nuse ./app\nreplace example.com/other => ./override\n"),
		alternate: map[string]string{filepath.Join(root.Name(), "app"): filepath.Join(root.Name(), "alternate", "build.mod")},
		files:     files,
	}
}

func TestCollectWorkspaceSources(t *testing.T) {
	t.Parallel()

	fixture := newWorkspaceSourceFixture(t)
	want := []string{filepath.Join(fixture.root.Name(), "app"), filepath.Join(fixture.root.Name(), "local"),
		filepath.Join(fixture.root.Name(), "override")}
	slices.Sort(want)

	for range 2 {
		got, err := compiler.CollectWorkspaceSources(t.Context(), fixture.workspace, fixture.root.Name(), fixture.alternate)
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("sources = %v, %v; want %v", got, err, want)
		}
	}

	for path, expected := range fixture.files {
		data, err := fixture.root.ReadFile(path)
		if err != nil || string(data) != expected {
			t.Fatal("source collection changed a manifest", err)
		}
	}

	_, err := fixture.root.Lstat("app/go.sum")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("collection created module state")
	}
}

func TestWorkspaceSourceCollectionCancellation(t *testing.T) {
	t.Parallel()

	fixture := newWorkspaceSourceFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := compiler.CollectWorkspaceSources(ctx, fixture.workspace, fixture.root.Name(), fixture.alternate)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation identity was not preserved", err)
	}
}

func TestWorkspaceSourceCollectionRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	fixture := newWorkspaceSourceFixture(t)
	for _, invalid := range [][]byte{[]byte("invalid"), []byte("go 1.25.0\n"), []byte("go 1.25.0\nuse ./missing\n")} {
		_, err := compiler.CollectWorkspaceSources(t.Context(), invalid, fixture.root.Name(), nil)
		if err == nil {
			t.Fatal("accepted invalid workspace")
		}
	}

	_, err := compiler.CollectWorkspaceSources(t.Context(), fixture.workspace, fixture.root.Name(), nil)
	if err == nil {
		t.Fatal("accepted missing local replacement")
	}

	unknown := filepath.Join(fixture.root.Name(), "unknown")
	fixture.alternate[unknown] = filepath.Join(fixture.root.Name(), "alternate", "build.mod")

	_, err = compiler.CollectWorkspaceSources(t.Context(), fixture.workspace, fixture.root.Name(), fixture.alternate)
	if err == nil {
		t.Fatal("accepted unknown alternate module")
	}
}

func TestCollectWorkspaceSourcesFilesystemAliases(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = root.WriteFile("go.mod", []byte("module example.com/app\nreplace example.com/alias => ./alias\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = root.Symlink(root.Name(), "alias")
	if err != nil {
		t.Fatal(err)
	}

	_, err = compiler.CollectWorkspaceSources(t.Context(), []byte("go 1.25.0\nuse .\n"), root.Name(), nil)
	if err == nil {
		t.Fatal("accepted ambiguous filesystem aliases")
	}
}

func TestWorkspaceSourceCollectionMissingDirectoryPreservesCause(t *testing.T) {
	t.Parallel()

	fixture := newWorkspaceSourceFixture(t)

	_, err := compiler.CollectWorkspaceSources(t.Context(), []byte("go 1.25.0\nuse ./missing\n"), fixture.root.Name(), nil)
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "resolve source module directory") {
		t.Fatal("missing directory lost its filesystem cause or operation context", err)
	}
}

func TestWorkspaceSourceCollectionMissingManifestPreservesCause(t *testing.T) {
	t.Parallel()

	fixture := newWorkspaceSourceFixture(t)

	err := fixture.root.Mkdir("without-manifest", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	work := []byte("go 1.25.0\nuse ./without-manifest\n")

	_, err = compiler.CollectWorkspaceSources(t.Context(), work, fixture.root.Name(), nil)
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "read source module manifest") {
		t.Fatal("missing manifest lost its filesystem cause or operation context", err)
	}
}

func TestWorkspaceSourceCollectionInvalidRootsShareCause(t *testing.T) {
	t.Parallel()

	_, first := compiler.CollectWorkspaceSources(t.Context(), nil, "relative", nil)

	_, second := compiler.CollectWorkspaceSources(t.Context(), nil, "another-relative", nil)
	if first == nil || !errors.Is(second, first) || first.Error() != "workspace directory must be absolute" {
		t.Fatal("invalid workspace roots lost their shared cause or meaningful message")
	}
}

func TestWorkspaceSourceCollectionAllowsOwnedExternalReplacement(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	external, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = external.Close() }()

	err = external.WriteFile("go.mod", []byte("module example.com/local\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	body := "module example.com/app\nreplace example.com/local => " + strconv.Quote(external.Name()) + "\n"

	err = root.WriteFile("go.mod", []byte(body), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	got, err := compiler.CollectWorkspaceSources(t.Context(), []byte("go 1.25.0\nuse .\n"), root.Name(), nil)
	want := []string{root.Name(), external.Name()}
	slices.Sort(want)

	if err != nil || !slices.Equal(got, want) {
		t.Fatal("explicit caller-owned external replacement was not collected", err)
	}
}
