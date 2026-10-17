package compiler_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
)

func TestPreviewBuildDestinationPreservesFilesAndMissingParents(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	err = root.WriteFile("existing", []byte("original binary"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"existing", filepath.Join("missing", "nested", "binary")} {
		destination := filepath.Join(root.Name(), name)

		got, previewErr := compiler.PreviewBuildDestination(destination)
		if previewErr != nil || got != destination {
			t.Fatalf("destination=%q error=%v; want %q", got, previewErr, destination)
		}
	}

	contents, err := root.ReadFile("existing")
	if err != nil || string(contents) != "original binary" {
		t.Fatalf("existing output changed: %q, %v", contents, err)
	}

	_, err = root.Lstat("missing")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preview created output parent: %v", err)
	}
}

func TestPreviewBuildDestinationRejectsUnsafePaths(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	err = root.WriteFile("file", []byte("preserved"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = root.Symlink("file", "link")
	if err != nil {
		t.Fatal(err)
	}

	err = root.Symlink("absent", "dangling")
	if err != nil {
		t.Fatal(err)
	}

	for _, destination := range []string{
		"", root.Name(), filepath.Join(root.Name(), "file", "binary"),
		filepath.Join(root.Name(), "link"), filepath.Join(root.Name(), "dangling", "binary"),
	} {
		_, previewErr := compiler.PreviewBuildDestination(destination)
		if previewErr == nil {
			t.Fatalf("preview accepted unsafe destination %q", destination)
		}
	}

	contents, err := root.ReadFile("file")
	if err != nil || string(contents) != "preserved" {
		t.Fatalf("preview changed source of rejected path: %q, %v", contents, err)
	}
}
