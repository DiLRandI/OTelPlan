package compiler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreparedBuildDirectory(t *testing.T) {
	source, parent := t.TempDir(), t.TempDir()
	copied := filepath.Join(parent, "module")
	nested := filepath.Join(parent, "nested")
	for _, dir := range []string{filepath.Join(copied, "cmd", "app"), filepath.Join(nested, "cmd")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	workspace := PreparedWorkspace{Dir: parent, Relocations: map[string]string{source: copied, filepath.Join(source, "nested"): nested}}
	for _, tc := range []struct{ original, want string }{{source, copied}, {filepath.Join(source, "cmd", "app"), filepath.Join(copied, "cmd", "app")}, {filepath.Join(source, "nested", "cmd"), filepath.Join(nested, "cmd")}} {
		got, err := workspace.BuildDirectory(tc.original)
		if err != nil || got != tc.want {
			t.Fatalf("build directory = %s, %v; want %s", got, err, tc.want)
		}
	}
	for _, path := range []string{"relative", source + "-other", filepath.Join(source, "missing")} {
		if _, err := workspace.BuildDirectory(path); err == nil {
			t.Fatal("accepted unknown working directory")
		}
	}
	for _, path := range []string{source, parent, t.TempDir()} {
		if err := workspace.validateBuildDirectory(path); err == nil {
			t.Fatal("accepted directory outside copied modules")
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(copied, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := workspace.BuildDirectory(filepath.Join(source, "escape")); err == nil {
		t.Fatal("accepted symlink escape")
	}
}
