package testenv

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"testing"
)

// SnapshotFiles records all regular fixture file bytes with owned root cleanup.
func SnapshotFiles(t *testing.T, root string) map[string][]byte {
	t.Helper()

	directory, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("open integration fixture: %v", err)
	}

	t.Cleanup(func() {
		err := directory.Close()
		if err != nil {
			t.Errorf("close integration fixture: %v", err)
		}
	})

	files := map[string][]byte{}

	err = fs.WalkDir(directory.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk integration fixture: %w", walkErr)
		}

		if entry.IsDir() {
			return nil
		}

		data, readErr := directory.ReadFile(name)
		if readErr != nil {
			return fmt.Errorf("read integration fixture: %w", readErr)
		}

		files[name] = data

		return nil
	})
	if err != nil {
		t.Fatalf("snapshot integration fixture: %v", err)
	}

	return files
}

// AssertFilesUnchanged rejects added, removed, or changed fixture files.
func AssertFilesUnchanged(t *testing.T, original, current map[string][]byte) {
	t.Helper()

	if len(original) != len(current) {
		t.Errorf("integration fixture file count changed: got %d, want %d", len(current), len(original))
	}

	for name, want := range original {
		got, exists := current[name]
		if !exists || !bytes.Equal(got, want) {
			t.Errorf("changed or removed project file %s", name)
		}
	}

	for name := range current {
		if _, exists := original[name]; !exists {
			t.Errorf("unexpected project file %s", name)
		}
	}
}
