package lockfile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
)

func TestWriteReportsReplacementFailureAndCleansTemporaryFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	destination := filepath.Join(directory, "lock.json")

	err := os.Mkdir(destination, 0o700)
	if err != nil {
		t.Fatalf("create destination directory: %v", err)
	}

	err = lockfile.Write(destination, fixtureLock(t))
	if err == nil || !strings.Contains(err.Error(), "replace lockfile:") {
		t.Fatalf("Write error = %v; want replacement failure", err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read fixture directory: %v", err)
	}

	if len(entries) != 1 || entries[0].Name() != "lock.json" || !entries[0].IsDir() {
		t.Fatalf("Write changed destination or left a temporary file: %v", entries)
	}
}
