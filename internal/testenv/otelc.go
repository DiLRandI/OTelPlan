package testenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// PathWithOTelC returns PATH with the requested backend exposed as otelc.
// It skips when OTELPLAN_OTELC is unset.
func PathWithOTelC(t *testing.T) string {
	t.Helper()

	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required for pinned backend integration")
	}

	path, err := exec.LookPath(executable)
	if err != nil {
		t.Fatalf("find requested integration backend: %v", err)
	}

	path, err = filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve requested integration backend: %v", err)
	}

	directory := t.TempDir()

	err = os.Symlink(path, filepath.Join(directory, "otelc"))
	if err != nil {
		t.Fatalf("expose requested integration backend: %v", err)
	}

	return directory + string(os.PathListSeparator) + os.Getenv("PATH")
}
