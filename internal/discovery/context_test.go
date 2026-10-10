package discovery

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestLoadCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := LoadContext(ctx, Options{Root: t.TempDir()}); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestIsolatedModuleMetadataOwnsPrivateDirectory(t *testing.T) {
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)
	t.Setenv("TMP", temporary)
	t.Setenv("TEMP", temporary)

	root := fixture(t, map[string]string{"app.go": "package shop\nfunc Run() {}\n"})
	original := filepath.Join(root, "go.mod")
	checksums := []byte("example.com/dependency v1.0.0 h1:fixture\n")

	err := os.WriteFile(filepath.Join(root, "go.sum"), checksums, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	target, cleanup, err := isolateModuleManifest(original)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(cleanup)

	directory := assertIsolatedModuleDirectory(t, target, temporary)
	assertIsolatedModuleCopies(t, original, target)

	cleanup()

	_, err = os.Stat(directory)
	if !os.IsNotExist(err) {
		t.Fatalf("cleanup left its owned directory: %v", err)
	}

	got := readOwnedModuleFixture(t, filepath.Join(root, "go.sum"))
	if !slices.Equal(got, checksums) {
		t.Fatal("cleanup changed original checksums")
	}
}

func assertIsolatedModuleDirectory(t *testing.T, target, temporary string) string {
	t.Helper()

	directory := filepath.Dir(target)
	if directory == temporary || filepath.Dir(directory) != temporary {
		t.Fatalf("module metadata is not in an owned private temporary directory: %s", target)
	}

	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("isolated directory permission=%#o; want 0700", info.Mode().Perm())
	}

	return directory
}

func assertIsolatedModuleCopies(t *testing.T, original, target string) {
	t.Helper()

	for _, pair := range [][2]string{{original, target}, {companionSum(original), companionSum(target)}} {
		want := readOwnedModuleFixture(t, pair[0])

		got := readOwnedModuleFixture(t, pair[1])
		if !slices.Equal(got, want) {
			t.Fatal("copied module metadata differs")
		}
	}
}

func readOwnedModuleFixture(t *testing.T, filename string) []byte {
	t.Helper()

	root, err := os.OpenRoot(filepath.Dir(filename))
	if err != nil {
		t.Fatal(err)
	}

	contents, readErr := root.ReadFile(filepath.Base(filename))

	closeErr := root.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read module fixture: read=%v close=%v", readErr, closeErr)
	}

	return contents
}

func TestIsolatedModuleMetadataCleansFailedChecksumRead(t *testing.T) {
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)
	t.Setenv("TMP", temporary)
	t.Setenv("TEMP", temporary)

	root := fixture(t, map[string]string{"app.go": "package shop\nfunc Run() {}\n"})

	err := os.Mkdir(filepath.Join(root, "go.sum"), 0o700)
	if err != nil {
		t.Fatal(err)
	}

	target, cleanup, err := isolateModuleManifest(filepath.Join(root, "go.mod"))
	if err == nil || !strings.Contains(err.Error(), "read effective module checksums") || target != "" || cleanup != nil {
		t.Fatalf("checksum failure returned usable metadata: target=%s error=%v", target, err)
	}

	entries, err := os.ReadDir(temporary)
	if err != nil || len(entries) != 0 {
		t.Fatalf("checksum failure leaked temporary metadata: entries=%v error=%v", entries, err)
	}
}

func TestIsolatedModuleChecksumsCannotEscapeRoot(t *testing.T) {
	t.Parallel()

	source := fixture(t, map[string]string{"app.go": "package shop\nfunc Run() {}\n"})

	err := os.WriteFile(filepath.Join(source, "go.sum"), []byte("fixture checksums"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	victim := filepath.Join(t.TempDir(), "keep")
	unchanged := []byte("outside the owned metadata directory")

	err = os.WriteFile(victim, unchanged, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	directory := t.TempDir()

	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := root.Close()
		if err != nil {
			t.Error(err)
		}
	})

	err = os.Symlink(victim, filepath.Join(directory, "effective.sum"))
	if err != nil {
		t.Fatalf("create checksum escape fixture: %v", err)
	}

	manifest := readOwnedModuleFixture(t, filepath.Join(source, "go.mod"))

	err = writeModuleMetadata(root, filepath.Join(source, "go.mod"), manifest)
	if err == nil || !strings.Contains(err.Error(), "write isolated module checksums") {
		t.Fatalf("checksum escape was not rejected: %v", err)
	}

	got := readOwnedModuleFixture(t, victim)
	if !slices.Equal(got, unchanged) {
		t.Fatal("isolated checksum write changed an outside file")
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
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/shop\n\ngo 1.27\n\nrequire example.com/dependency v1.0.0\n"), 0o644); err != nil {
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

		err := os.MkdirAll(filepath.Dir(filename), 0o755)
		if err != nil {
			t.Fatal(err)
		}

		err = os.WriteFile(filename, []byte(contents), 0o644)
		if err != nil {
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

func TestExplicitModuleModes(t *testing.T) {
	for _, mode := range []string{"mod", "readonly", "vendor"} {
		t.Run(mode, func(t *testing.T) {
			root := fixture(t, map[string]string{"app.go": "package shop\nfunc Run() {}\n", "vendor/modules.txt": ""})
			opts := Options{Root: root, Env: []string{"GOWORK=off", "GOFLAGS=-mod=" + mode}}

			_, flags, err := prepare(t.Context(), &opts)
			if err != nil {
				t.Fatal(err)
			}

			defer opts.cleanup()

			if flags[0] != "-mod="+mode || opts.effectiveBuild.ModuleMode != mode {
				t.Fatalf("explicit mode overwritten: %v %+v", flags, opts.effectiveBuild)
			}
		})
	}
}

func TestGOFLAGSPrecedenceAndQuoting(t *testing.T) {
	for _, tc := range []struct {
		raw, mode   string
		tags, flags []string
	}{
		{raw: "--mod=mod -mod=readonly", mode: "readonly"},
		{raw: "-tags=old '-tags=new,other'", tags: []string{"new", "other"}},
		{raw: "-race -race=false", flags: []string{"-race=false"}},
		{raw: `'-modfile=C:\project\alternate.mod'`},
	} {
		got, err := parseGOFLAGS(tc.raw)
		if err != nil {
			t.Fatal(err)
		}

		if got.moduleMode != tc.mode || !slices.Equal(got.tags, tc.tags) || !slices.Equal(got.semantic, tc.flags) {
			t.Fatalf("parse %q: %+v", tc.raw, got)
		}
	}

	for _, raw := range []string{"-mod mod", "'-tags=broken", "-overlay=private-path", "-toolexec=private-command"} {
		if _, err := parseGOFLAGS(raw); err == nil || strings.Contains(err.Error(), "private-") {
			t.Fatalf("invalid flags not safely rejected: %v", err)
		}
	}
}
