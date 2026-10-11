package discovery_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestLoadCanceled(t *testing.T) {
	t.Parallel()

	root, directory, files := buildContextFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	code, err := discovery.LoadContext(ctx, options)
	if err == nil || code != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled load result=%+v error=%v", code, err)
	}

	checkLoaderFixtureFiles(t, directory, files)
}

func TestLoadRespectsEnvironment(t *testing.T) {
	t.Setenv("GOARCH", "386")

	root, directory, files := buildContextFixture(t)

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	code := loadEnvironmentContext(t, options)
	if code.GOARCH != "386" || code.EffectiveBuild.GOARCH != "386" {
		t.Fatalf("GOARCH=%s effective=%s, ignored environment", code.GOARCH, code.EffectiveBuild.GOARCH)
	}

	checkLoaderFixtureFiles(t, directory, files)
}

func TestLoadVendorWithoutNetwork(t *testing.T) {
	t.Setenv("GOPROXY", "off")

	files := map[string]string{
		"go.mod":             "module example.com/shop\n\ngo 1.27\n\nrequire example.com/dependency v1.0.0\n",
		"app.go":             "package shop\nimport _ \"example.com/dependency\"\nfunc Run() {}\n",
		"vendor/modules.txt": "# example.com/dependency v1.0.0\n## explicit; go 1.27\nexample.com/dependency\n",
		"vendor/example.com/dependency/dependency.go": "package dependency\nfunc Dependency() {}\n",
	}
	root, directory, original := loaderFixture(t, files)

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	code := loadEnvironmentContext(t, options)
	if code.EffectiveBuild.ModuleMode != "vendor" {
		t.Fatalf("module mode=%s, want vendor", code.EffectiveBuild.ModuleMode)
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

	if _, exists := code.Symbol("example.com/dependency.Dependency"); exists {
		t.Fatal("dependency symbol selected without opt-in")
	}

	_, statErr := directory.Stat("go.sum")
	if !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("analysis created a go.sum: %v", statErr)
	}

	checkLoaderFixtureFiles(t, directory, original)
}

func TestLoadWorkspaceRoot(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"go.work":  "go 1.27\n\nuse (\n ./a\n ./b\n)\n",
		"a/go.mod": "module example.com/a\n\ngo 1.27\n",
		"a/app.go": "package a\nfunc A() {}\n",
		"b/go.mod": "module example.com/b\n\ngo 1.27\n",
		"b/app.go": "package b\nfunc B() {}\n",
	}
	root, directory, original := loaderFixture(t, files)

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Env = []string{"GOWORK=" + filepath.Join(root, "go.work"), "GOFLAGS=", "CGO_ENABLED=0"}

	code := loadEnvironmentContext(t, options)
	if len(code.Modules) != 2 {
		t.Fatalf("workspace modules: %+v", code.Modules)
	}

	if _, exists := code.Symbol("example.com/a.A"); !exists {
		t.Fatal("module a missing")
	}

	if _, exists := code.Symbol("example.com/b.B"); !exists {
		t.Fatal("module b missing")
	}

	checkLoaderFixtureFiles(t, directory, original)
}

func loadEnvironmentContext(t *testing.T, options discovery.Options) *model.CodeModel {
	t.Helper()

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	return code
}

func loaderFixture(t *testing.T, files map[string]string) (string, *os.Root, map[string]string) {
	t.Helper()

	root := t.TempDir()

	directory, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := directory.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	})

	for name, contents := range files {
		err = directory.MkdirAll(path.Dir(name), 0o700)
		if err != nil {
			t.Fatal(err)
		}

		err = directory.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	return root, directory, files
}

func checkLoaderFixtureFiles(t *testing.T, directory *os.Root, files map[string]string) {
	t.Helper()

	seen := make(map[string]bool, len(files))

	err := fs.WalkDir(directory.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			return nil
		}

		seen[name] = true

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(seen) != len(files) {
		t.Fatalf("fixture file set changed: got %d files, want %d", len(seen), len(files))
	}

	for name, want := range files {
		if !seen[name] {
			t.Fatalf("fixture file missing: %s", name)
		}

		got, readErr := directory.ReadFile(name)
		if readErr != nil || !bytes.Equal(got, []byte(want)) {
			t.Fatalf("fixture file changed: %s", name)
		}
	}
}
