package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLockCheckIgnoresSourceCoordinates(t *testing.T) {
	root, files := cliFixture(t)
	invoke(t, root, 0, "lock")
	filename := filepath.Join(root, "app.go")
	moved := strings.Replace(files["app.go"], "func Run", "\n\n\tfunc Run", 1)
	if err := os.WriteFile(filename, []byte(moved), 0600); err != nil {
		t.Fatal(err)
	}
	invoke(t, root, 0, "lock", "--check")
	invoke(t, root, 0, "diff", "--check")
	if err := os.Rename(filename, filepath.Join(root, "moved.go")); err != nil {
		t.Fatal(err)
	}
	invoke(t, root, 6, "lock", "--check")
}
