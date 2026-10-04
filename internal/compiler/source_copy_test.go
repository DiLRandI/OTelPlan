package compiler_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
)

func copySourceFixture(t *testing.T) (*os.Root, map[string]string) {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	files := map[string]string{
		"go.mod": "module example.com/copied\n\ngo 1.25.0\n",
		"main.go": "package main\nimport (\n_ \"embed\"\n\"fmt\"\n)\n" +
			"//go:embed assets/message.txt\nvar message string\nfunc main(){fmt.Print(message)}\n",
		"assets/message.txt": "copied asset",
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

	return root, files
}

func assertCopySourceUnchanged(t *testing.T, root *os.Root, files map[string]string) {
	t.Helper()

	for path, expected := range files {
		contents, err := root.ReadFile(path)
		if err != nil || string(contents) != expected {
			t.Fatalf("original source changed at %s: %v", path, err)
		}
	}
}

func TestCopySourceTreeBuild(t *testing.T) {
	t.Parallel()

	source, files := copySourceFixture(t)

	err := source.Symlink(filepath.Join(source.Name(), "assets"), "linked-assets")
	if err != nil {
		t.Fatal(err)
	}

	copied, err := compiler.CopySourceTree(t.Context(), source.Name(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	linked, err := filepath.EvalSymlinks(filepath.Join(copied, "linked-assets"))
	if err != nil || linked != filepath.Join(copied, "assets") {
		t.Fatalf("link not relocated: %s, %v", linked, err)
	}

	command := exec.CommandContext(t.Context(), "go", "run", "-buildvcs=false", "-mod=readonly", ".")

	command.Dir, command.Env = copied, append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOPROXY=off")

	output, err := command.CombinedOutput()
	if err != nil || string(output) != "copied asset" {
		t.Fatalf("copied build/run failed: %v, %s", err, output)
	}

	root, err := os.OpenRoot(copied)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = root.WriteFile("linked-assets/message.txt", []byte("changed"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	assertCopySourceUnchanged(t, source, files)
}

func TestCopySourceTreeRejectsExternalLinks(t *testing.T) {
	t.Parallel()

	source, files := copySourceFixture(t)
	parent := t.TempDir()

	err := source.Symlink(t.TempDir(), "z-external")
	if err != nil {
		t.Fatal(err)
	}

	_, err = compiler.CopySourceTree(t.Context(), source.Name(), parent)
	if err == nil {
		t.Fatal("accepted external link")
	}

	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("failed copy left partial output", err)
	}

	assertCopySourceUnchanged(t, source, files)
}

func TestCopySourceTreeCancellationAndNestedOutput(t *testing.T) {
	t.Parallel()

	source, parent := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := compiler.CopySourceTree(ctx, source, parent)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("ignored cancellation", err)
	}

	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("cancelled copy left output", err)
	}

	_, err = compiler.CopySourceTree(t.Context(), source, source)
	if err == nil {
		t.Fatal("accepted output within source")
	}
}

func TestNestedSourceStagingErrorsShareIdentity(t *testing.T) {
	t.Parallel()

	source := t.TempDir()
	_, first := compiler.CopySourceTree(t.Context(), source, source)

	_, second := compiler.CopySourceTree(t.Context(), source, source)
	if first == nil || !errors.Is(second, first) || first.Error() != "source staging parent must be outside source tree" {
		t.Fatal("invalid staging roots lost their shared cause or meaningful message")
	}
}

func TestSourceCopyUsesPrivateExecutablePermissions(t *testing.T) {
	t.Parallel()

	source, _ := copySourceFixture(t)

	err := source.WriteFile("executable", []byte("executable fixture"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = source.Chmod("executable", 0o755)
	if err != nil {
		t.Fatal(err)
	}

	copied, err := compiler.CopySourceTree(t.Context(), source.Name(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(copied)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	for name, mode := range map[string]fs.FileMode{"executable": 0o700, "main.go": 0o600, "assets": 0o700} {
		info, err := root.Stat(name)
		if err != nil {
			t.Fatal(err)
		}

		if runtime.GOOS != "windows" && info.Mode().Perm() != mode {
			t.Fatalf("copy permissions for %s = %o; want %o", name, info.Mode().Perm(), mode)
		}
	}
}
