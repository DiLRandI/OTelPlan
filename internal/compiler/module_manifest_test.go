package compiler_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"golang.org/x/mod/modfile"
)

func TestRelocateModuleManifest(t *testing.T) {
	t.Parallel()

	original := t.TempDir()
	local := filepath.Join(original, "dep")
	copied := filepath.Join(t.TempDir(), "copied dep")
	data := []byte("module example.com/app\n\ngo 1.25.0\nrequire example.com/dep v1.0.0\n" +
		"replace example.com/dep v1.0.0 => ./dep\nreplace example.com/other => example.com/fork v1.2.0\n")
	before := bytes.Clone(data)

	result, err := compiler.RelocateModuleManifest(data, original, map[string]string{local: copied})
	if err != nil {
		t.Fatal(err)
	}

	file, err := modfile.Parse("go.mod", result, nil)
	if err != nil {
		t.Fatal(err)
	}

	assertManifestReplacementSemantics(t, file, copied)

	if file.Require[0].Mod.Version != "v1.0.0" || file.Go.Version != "1.25.0" || !bytes.Equal(data, before) {
		t.Fatal("mutated unrelated module state")
	}

	repeated, err := compiler.RelocateModuleManifest(data, original, map[string]string{local: copied})
	if err != nil || !bytes.Equal(result, repeated) {
		t.Fatal("nondeterministic relocation")
	}

	output, err := compiler.RelocateModuleManifest(data, original, nil)
	if err == nil || output != nil {
		t.Fatal("accepted uncopied local replacement")
	}
}

func assertManifestReplacementSemantics(t *testing.T, file *modfile.File, copied string) {
	t.Helper()

	if file.Replace[0].New.Path != copied || file.Replace[0].Old.Version != "v1.0.0" {
		t.Fatal("local replacement semantics changed")
	}

	if file.Replace[1].New.Path != "example.com/fork" || file.Replace[1].New.Version != "v1.2.0" {
		t.Fatal("versioned replacement semantics changed")
	}
}

func relocatedModuleSource() map[string]string {
	return map[string]string{
		"app/go.mod": "module example.com/app\n\ngo 1.25.0\n" +
			"require example.com/dep v1.0.0\nreplace example.com/dep => ../dep\n",
		"app/main.go": "package main\nimport (\"fmt\"; \"example.com/dep\")\nfunc main(){fmt.Print(dep.Value)}\n",
		"dep/go.mod":  "module example.com/dep\n\ngo 1.25.0\n",
		"dep/dep.go":  "package dep\nconst Value = \"isolated\"\n",
	}
}

func writeRelocationFixture(t *testing.T, root *os.Root, files map[string]string) {
	t.Helper()

	for name, data := range files {
		filename := filepath.FromSlash(name)

		err := root.MkdirAll(filepath.Dir(filename), 0o700)
		if err != nil {
			t.Fatal(err)
		}

		err = root.WriteFile(filename, []byte(data), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildRelocatedModule(t *testing.T) {
	t.Parallel()

	original := t.TempDir()
	application, dependency := filepath.Join(original, "app"), filepath.Join(original, "dep")

	root, err := os.OpenRoot(original)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	source := relocatedModuleSource()
	writeRelocationFixture(t, root, source)
	parent := t.TempDir()

	appCopy, err := compiler.CopySourceTree(t.Context(), application, parent)
	if err != nil {
		t.Fatal(err)
	}

	depCopy, err := compiler.CopySourceTree(t.Context(), dependency, parent)
	if err != nil {
		t.Fatal(err)
	}

	mapping := map[string]string{dependency: depCopy}

	manifest, err := compiler.RelocateModuleManifest([]byte(source["app/go.mod"]), application, mapping)
	if err != nil {
		t.Fatal(err)
	}

	appRoot, err := os.OpenRoot(appCopy)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = appRoot.Close() }()

	err = appRoot.WriteFile("go.mod", manifest, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	// Remove the original dependency location to prove the build uses its isolated copy.
	err = root.Rename("dep", "original-dep")
	if err != nil {
		t.Fatal(err)
	}

	runRelocatedModule(t, appCopy)

	originalManifest, err := root.ReadFile("app/go.mod")
	if err != nil || string(originalManifest) != source["app/go.mod"] {
		t.Fatal("original manifest changed")
	}
}

func runRelocatedModule(t *testing.T, directory string) {
	t.Helper()

	command := exec.CommandContext(t.Context(), "go", "run", "-mod=readonly", ".")
	command.Dir = directory

	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOPROXY=off")

	output, err := command.CombinedOutput()
	if err != nil || string(output) != "isolated" {
		t.Fatalf("isolated replacement build failed: %v\n%s", err, output)
	}
}

func TestManifestRelocationErrorsShareIdentity(t *testing.T) {
	t.Parallel()

	_, first := compiler.RelocateModuleManifest(nil, ".", nil)

	_, second := compiler.RelocateModuleManifest(nil, "relative", nil)
	if first == nil || !errors.Is(second, first) || first.Error() != "original module directory must be absolute" {
		t.Fatal("invalid origin lost its shared cause or meaningful message")
	}
}

func TestManifestRelocationRejectsRelativeCopies(t *testing.T) {
	t.Parallel()

	original := t.TempDir()
	data := []byte("module example.com/app\nreplace example.com/dep => ./dep\n")
	copies := map[string]string{filepath.Join(original, "dep"): "relative"}

	result, err := compiler.RelocateModuleManifest(data, original, copies)
	if err == nil || result != nil || err.Error() != "local module replacement has no isolated copy" {
		t.Fatal("accepted an unverified relative replacement or lost error context", err)
	}
}
