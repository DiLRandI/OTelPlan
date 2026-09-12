package compiler

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCopySourceTreeBuild(t *testing.T) {
	source := t.TempDir()
	files := map[string]string{
		"go.mod":             "module example.com/copied\n\ngo 1.25.0\n",
		"main.go":            "package main\nimport (\n_ \"embed\"\n\"fmt\"\n)\n//go:embed assets/message.txt\nvar message string\nfunc main(){fmt.Print(message)}\n",
		"assets/message.txt": "copied asset",
	}
	for path, data := range files {
		name := filepath.Join(source, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(source, "assets"), filepath.Join(source, "linked-assets")); err != nil {
		t.Fatal(err)
	}
	copied, err := CopySourceTree(t.Context(), source, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	linked, err := filepath.EvalSymlinks(filepath.Join(copied, "linked-assets"))
	if err != nil || linked != filepath.Join(copied, "assets") {
		t.Fatalf("link not relocated: %s, %v", linked, err)
	}
	binary := filepath.Join(t.TempDir(), "app")
	build := exec.CommandContext(t.Context(), "go", "build", "-mod=readonly", "-o", binary, ".")
	build.Dir = copied
	build.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOPROXY=off")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("copied build failed: %v\n%s", err, output)
	}
	output, err := exec.CommandContext(t.Context(), binary).Output()
	if err != nil || string(output) != "copied asset" {
		t.Fatalf("copied binary failed: %v, %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(copied, "linked-assets", "message.txt"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	for path, wanted := range files {
		actual, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(path)))
		if err != nil || string(actual) != wanted {
			t.Fatalf("original file changed: %s", path)
		}
	}
}

func TestCopySourceTreeRejectsExternalLinks(t *testing.T) {
	source, parent := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "a"), []byte("already copied"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(source, "z")); err != nil {
		t.Fatal(err)
	}
	if _, err := CopySourceTree(t.Context(), source, parent); err == nil {
		t.Fatal("accepted external link")
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("failed copy left partial output")
	}
}

func TestCopySourceTreeCancellationAndNestedOutput(t *testing.T) {
	source, parent := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := CopySourceTree(ctx, source, parent); err == nil {
		t.Fatal("ignored cancellation")
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("cancelled copy left output")
	}
	if _, err := CopySourceTree(t.Context(), source, source); err == nil {
		t.Fatal("accepted output within source")
	}
}
