package compiler

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterializeVendorWorkspace(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"app/go.mod":     "module example.com/app\n\ngo 1.27\nrequire example.com/dep v0.0.0\nreplace example.com/dep => ../dep\n",
		"app/app.go":     "package app\nimport \"example.com/dep\"\nfunc Name() string { return dep.Name() }\n",
		"dep/go.mod":     "module example.com/dep\n\ngo 1.27\n",
		"dep/dep.go":     "package dep\nfunc Name() string { return \"original\" }\n",
		"runtime/go.mod": "module example.com/runtime\n\ngo 1.27\nrequire example.com/dep v0.0.0\nreplace example.com/dep => ../dep\n",
		"runtime/run.go": "package runtime\nimport \"example.com/dep\"\nfunc Name() string { return dep.Name() }\n",
		"go.work":        "go 1.27\nuse (\n./app\n./runtime\n)\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	app := filepath.Join(root, "app")
	command := exec.CommandContext(t.Context(), "go", "mod", "vendor")
	command.Dir = app

	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOPROXY=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("prepare fixture vendor tree: %v\n%s", err, output)
	}

	originalVendor := filepath.Join(app, "vendor")
	originalFile := filepath.Join(originalVendor, "example.com", "dep", "dep.go")
	before, err := os.ReadFile(originalFile)
	if err != nil {
		t.Fatal(err)
	}

	workspace := PreparedWorkspace{Dir: root, WorkspaceFile: filepath.Join(root, "go.work")}

	env := append(os.Environ(), "GOFLAGS=", "GOPROXY=off")
	if err := materializeVendorWorkspace(t.Context(), workspace, originalVendor, env); err != nil {
		t.Fatal(err)
	}

	command = exec.CommandContext(t.Context(), "go", "list", "-mod=vendor", "./...")
	command.Dir = app
	command.Env = append(env, "GOWORK="+workspace.WorkspaceFile)
	if output, err := command.CombinedOutput(); err != nil || !strings.Contains(string(output), "example.com/app") {
		t.Fatalf("isolated vendor mode failed: %v\n%s", err, output)
	}

	after, err := os.ReadFile(originalFile)
	if err != nil || string(after) != string(before) {
		t.Fatal("materialization changed the analyzed vendor tree")
	}

	extraFile := filepath.Join(root, "dep", "extra.go")
	if err := os.WriteFile(extraFile, []byte("package dep\nfunc Extra() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := materializeVendorWorkspace(t.Context(), workspace, originalVendor, env); err == nil || !strings.Contains(err.Error(), "was added") {
		t.Fatalf("accepted an added file in an analyzed vendored package: %v", err)
	}
	if err := os.Remove(extraFile); err != nil {
		t.Fatal(err)
	}
	if err := materializeVendorWorkspace(t.Context(), workspace, originalVendor, env); err != nil {
		t.Fatal(err)
	}

	workspaceCopy, err := CopySourceTree(t.Context(), root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	workspaceVendor := filepath.Join(root, "vendor")
	if err := materializeVendorWorkspace(t.Context(), PreparedWorkspace{Dir: workspaceCopy, WorkspaceFile: filepath.Join(workspaceCopy, "go.work")}, workspaceVendor, env); err != nil {
		t.Fatalf("prepare vendored multi-module workspace: %v", err)
	}

	if err := os.WriteFile(originalFile, []byte("package dep\nfunc Name() string { return \"patched\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := materializeVendorWorkspace(t.Context(), workspace, originalVendor, env); err == nil || !strings.Contains(err.Error(), "vendor content differs") {
		t.Fatalf("accepted vendor source differing from the isolated build: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := materializeVendorWorkspace(ctx, workspace, originalVendor, env); !errors.Is(err, context.Canceled) {
		t.Fatalf("vendor preparation ignored cancellation: %v", err)
	}
}
