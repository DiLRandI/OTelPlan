package discovery

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

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

func TestWorkspaceChecksumsCannotEscapeRoot(t *testing.T) {
	t.Parallel()

	source := t.TempDir()
	original := filepath.Join(source, "go.work")

	err := os.WriteFile(original+".sum", []byte("fixture workspace checksums"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	victim := filepath.Join(t.TempDir(), "keep")
	unchanged := []byte("outside the owned workspace directory")

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

	err = os.Symlink(victim, filepath.Join(directory, "go.work.sum"))
	if err != nil {
		t.Fatalf("create workspace checksum escape fixture: %v", err)
	}

	err = writeWorkspaceMetadata(root, original, []byte("go 1.27\n"))
	if err == nil || !strings.Contains(err.Error(), "write isolated workspace checksums") {
		t.Fatalf("workspace checksum escape was not rejected: %v", err)
	}

	got := readOwnedModuleFixture(t, victim)
	if !slices.Equal(got, unchanged) {
		t.Fatal("isolated workspace checksum write changed an outside file")
	}
}

func TestExplicitModuleModes(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"mod", "readonly", "vendor"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			root := fixture(t, map[string]string{"app.go": "package shop\nfunc Run() {}\n", "vendor/modules.txt": ""})

			var options Options

			options.Root = root
			options.Env = []string{"GOWORK=off", "GOFLAGS=-mod=" + mode}

			_, flags, err := prepare(t.Context(), &options)
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(options.cleanup)

			if len(flags) == 0 || flags[0] != "-mod="+mode || options.effectiveBuild.ModuleMode != mode {
				t.Fatalf("explicit mode overwritten: %v %+v", flags, options.effectiveBuild)
			}
		})
	}
}

func TestGOFLAGSPrecedenceAndQuoting(t *testing.T) {
	t.Parallel()

	valid := []struct {
		name                string
		raw, mode, modFile  string
		tags, semanticFlags []string
	}{
		{
			name: "module mode precedence", raw: "--mod=mod -mod=readonly", mode: "readonly",
			modFile: "", tags: nil, semanticFlags: nil,
		},
		{
			name: "quoted tags", raw: "-tags=old '-tags=new,other'", mode: "", modFile: "",
			tags: []string{"new", "other"}, semanticFlags: nil,
		},
		{
			name: "boolean precedence", raw: "-race -race=false", mode: "", modFile: "", tags: nil,
			semanticFlags: []string{"-race=false"},
		},
		{
			name: "windows modfile", raw: `'-modfile=C:\project\alternate.mod'`, mode: "",
			modFile: `C:\project\alternate.mod`, tags: nil, semanticFlags: nil,
		},
	}
	for _, validCase := range valid {
		t.Run(validCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseGOFLAGS(validCase.raw)
			if err != nil {
				t.Fatal(err)
			}

			if got.moduleMode != validCase.mode || got.modFile != validCase.modFile ||
				!slices.Equal(got.tags, validCase.tags) || !slices.Equal(got.semantic, validCase.semanticFlags) {
				t.Fatalf("parse %q: %+v", validCase.raw, got)
			}
		})
	}

	invalid := []struct {
		name, raw string
	}{
		{name: "module mode missing equals", raw: "-mod mod"},
		{name: "unterminated quote", raw: "'-tags=broken"},
		{name: "overlay", raw: "-overlay=private-path"},
		{name: "tool execution", raw: "-toolexec=private-command"},
	}
	for _, invalidCase := range invalid {
		t.Run(invalidCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseGOFLAGS(invalidCase.raw)
			if err == nil || strings.Contains(err.Error(), "private-") {
				t.Fatalf("invalid flags not safely rejected: %v", err)
			}
		})
	}
}
