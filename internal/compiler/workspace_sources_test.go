package compiler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestCollectWorkspaceSources(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("app/go.mod", "module example.com/app\nreplace example.com/ignored => ../missing\n")
	write("alternate/build.mod", "module example.com/app\nreplace example.com/local => ../local\nreplace example.com/versioned => example.com/remote v1.0.0\n")
	write("local/go.mod", "module example.com/local\nreplace example.com/app => ../app\n")
	write("override/go.mod", "module example.com/override\n")
	work := []byte("go 1.25.0\nuse ./app\nreplace example.com/other => ./override\n")
	alternate := map[string]string{filepath.Join(root, "app"): filepath.Join(root, "alternate", "build.mod")}
	want := []string{filepath.Join(root, "app"), filepath.Join(root, "local"), filepath.Join(root, "override")}
	slices.Sort(want)
	for range 2 {
		got, err := CollectWorkspaceSources(t.Context(), work, root, alternate)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("sources = %v, %v; want %v", got, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "app", "go.sum")); !os.IsNotExist(err) {
		t.Fatal("collection created module state")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := CollectWorkspaceSources(ctx, work, root, alternate); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	for _, invalid := range [][]byte{[]byte("invalid"), []byte("go 1.25.0\n"), []byte("go 1.25.0\nuse ./missing\n")} {
		if _, err := CollectWorkspaceSources(t.Context(), invalid, root, nil); err == nil {
			t.Fatal("accepted invalid workspace")
		}
	}
	if _, err := CollectWorkspaceSources(t.Context(), work, root, nil); err == nil {
		t.Fatal("accepted missing local replacement")
	}
	alternate[filepath.Join(root, "unknown")] = filepath.Join(root, "alternate", "build.mod")
	if _, err := CollectWorkspaceSources(t.Context(), work, root, alternate); err == nil {
		t.Fatal("accepted unknown alternate module")
	}
}

func TestCollectWorkspaceSourcesFilesystemAliases(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\nreplace example.com/alias => ./alias\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "alias")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := CollectWorkspaceSources(t.Context(), []byte("go 1.25.0\nuse .\n"), root, nil); err == nil {
		t.Fatal("accepted ambiguous filesystem aliases")
	}
}
