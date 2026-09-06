package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := LoadContext(ctx, Options{Root: t.TempDir()}); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestLoadRespectsEnvironment(t *testing.T) {
	t.Setenv("GOARCH", "386")
	root := fixture(t, map[string]string{"app.go": "package shop\nfunc Run() {}\n"})
	code, err := Load(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if code.GOARCH != "386" {
		t.Fatalf("GOARCH=%s, ignored environment", code.GOARCH)
	}
}

func TestLoadVendorWithoutNetwork(t *testing.T) {
	t.Setenv("GOPROXY", "off")
	root := fixture(t, map[string]string{
		"app.go":             "package shop\nimport _ \"example.com/dependency\"\nfunc Run() {}\n",
		"vendor/modules.txt": "# example.com/dependency v1.0.0\n## explicit; go 1.27\nexample.com/dependency\n",
		"vendor/example.com/dependency/dependency.go": "package dependency\nfunc Dependency() {}\n",
	})
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/shop\n\ngo 1.27\n\nrequire example.com/dependency v1.0.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	code, err := Load(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, module := range code.Modules {
		if module.Path == "example.com/dependency" {
			found = true
		}
	}
	if !found {
		t.Fatal("dependency module omitted from build metadata")
	}
	if _, ok := code.Symbol("example.com/dependency.Dependency"); ok {
		t.Fatal("dependency symbol selected without opt-in")
	}
	if _, err := os.Stat(filepath.Join(root, "go.sum")); !os.IsNotExist(err) {
		t.Fatal("analysis created a go.sum")
	}
}

func TestLoadWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.work":  "go 1.27\n\nuse (\n ./a\n ./b\n)\n",
		"a/go.mod": "module example.com/a\n\ngo 1.27\n",
		"a/app.go": "package a\nfunc A() {}\n",
		"b/go.mod": "module example.com/b\n\ngo 1.27\n",
		"b/app.go": "package b\nfunc B() {}\n",
	}
	for name, contents := range files {
		filename := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	code, err := Load(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(code.Modules) != 2 {
		t.Fatalf("workspace modules: %+v", code.Modules)
	}
	if _, ok := code.Symbol("example.com/a.A"); !ok {
		t.Fatal("module a missing")
	}
	if _, ok := code.Symbol("example.com/b.B"); !ok {
		t.Fatal("module b missing")
	}
}
