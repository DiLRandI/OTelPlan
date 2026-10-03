package compiler_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
)

func TestPreparedDirectoryMissingCopyPreservesCause(t *testing.T) {
	t.Parallel()

	source, parent := t.TempDir(), t.TempDir()
	workspace := new(compiler.PreparedWorkspace)
	workspace.Dir = parent
	workspace.Relocations = map[string]string{source: filepath.Join(parent, "missing")}

	directory, err := workspace.BuildDirectory(source)
	if directory != "" || !errors.Is(err, fs.ErrNotExist) ||
		!strings.Contains(err.Error(), "resolve prepared build directory") {
		t.Fatal("missing copied directory lost its filesystem cause or operation context", err)
	}
}

func TestPreparedDirectoryMissingWorkspacePreservesCause(t *testing.T) {
	t.Parallel()

	source, copied, parent := t.TempDir(), t.TempDir(), t.TempDir()
	workspace := new(compiler.PreparedWorkspace)
	workspace.Dir = filepath.Join(parent, "missing")
	workspace.Relocations = map[string]string{source: copied}

	directory, err := workspace.BuildDirectory(source)
	if directory != "" || !errors.Is(err, fs.ErrNotExist) ||
		!strings.Contains(err.Error(), "resolve prepared workspace directory") {
		t.Fatal("missing workspace lost its filesystem cause or operation context", err)
	}
}

func TestPreparedDirectoryOriginSharesErrorIdentity(t *testing.T) {
	t.Parallel()

	workspace := new(compiler.PreparedWorkspace)
	_, first := workspace.BuildDirectory(".")

	_, second := workspace.BuildDirectory("relative")
	if first == nil || !errors.Is(second, first) || first.Error() != "original build directory must be absolute" {
		t.Fatal("invalid origin errors lost their shared cause or meaningful message")
	}
}

func TestPreparedDirectoryRejectsFile(t *testing.T) {
	t.Parallel()

	source, parent := t.TempDir(), t.TempDir()

	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = root.WriteFile("file", []byte("not a directory"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	workspace := new(compiler.PreparedWorkspace)
	workspace.Dir, workspace.Relocations = parent, map[string]string{source: filepath.Join(parent, "file")}

	directory, err := workspace.BuildDirectory(source)
	if err == nil || directory != "" {
		t.Fatal("accepted a file as a prepared working directory")
	}
}
