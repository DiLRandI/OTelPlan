package compiler

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/mod/modfile"
)

func TestRelocateModuleManifest(t *testing.T) {
	original := t.TempDir()
	local := filepath.Join(original, "dep")
	copied := filepath.Join(t.TempDir(), "copied dep")
	data := []byte("module example.com/app\n\ngo 1.25.0\nrequire example.com/dep v1.0.0\nreplace example.com/dep v1.0.0 => ./dep\nreplace example.com/other => example.com/fork v1.2.0\n")
	before := append([]byte(nil), data...)
	result, err := RelocateModuleManifest(data, original, map[string]string{local: copied})
	if err != nil {
		t.Fatal(err)
	}
	file, err := modfile.Parse("go.mod", result, nil)
	if err != nil {
		t.Fatal(err)
	}
	if file.Replace[0].New.Path != copied || file.Replace[0].Old.Version != "v1.0.0" || file.Replace[1].New.Path != "example.com/fork" || file.Replace[1].New.Version != "v1.2.0" {
		t.Fatal("replacement semantics changed")
	}
	if file.Require[0].Mod.Version != "v1.0.0" || file.Go.Version != "1.25.0" || !bytes.Equal(data, before) {
		t.Fatal("mutated unrelated module state")
	}
	repeated, err := RelocateModuleManifest(data, original, map[string]string{local: copied})
	if err != nil || !bytes.Equal(result, repeated) {
		t.Fatal("nondeterministic relocation")
	}
	if output, err := RelocateModuleManifest(data, original, nil); err == nil || output != nil {
		t.Fatal("accepted uncopied local replacement")
	}
}

func TestBuildRelocatedModule(t *testing.T) {
	original := t.TempDir()
	application := filepath.Join(original, "app")
	dependency := filepath.Join(original, "dep")
	source := map[string]string{
		"app/go.mod":  "module example.com/app\n\ngo 1.25.0\nrequire example.com/dep v1.0.0\nreplace example.com/dep => ../dep\n",
		"app/main.go": "package main\nimport (\"fmt\"; \"example.com/dep\")\nfunc main(){fmt.Print(dep.Value)}\n",
		"dep/go.mod":  "module example.com/dep\n\ngo 1.25.0\n",
		"dep/dep.go":  "package dep\nconst Value = \"isolated\"\n",
	}
	for name, data := range source {
		filename := filepath.Join(original, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	parent := t.TempDir()
	appCopy, err := CopySourceTree(t.Context(), application, parent)
	if err != nil {
		t.Fatal(err)
	}
	depCopy, err := CopySourceTree(t.Context(), dependency, parent)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := RelocateModuleManifest([]byte(source["app/go.mod"]), application, map[string]string{dependency: depCopy})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appCopy, "go.mod"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	// Remove the original dependency from its old location to prove the build uses the copy.
	if err := os.Rename(dependency, filepath.Join(original, "original-dep")); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "go", "run", "-mod=readonly", ".")
	command.Dir = appCopy
	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOPROXY=off")
	output, err := command.CombinedOutput()
	if err != nil || string(output) != "isolated" {
		t.Fatalf("isolated replacement build failed: %v\n%s", err, output)
	}
	originalManifest, err := os.ReadFile(filepath.Join(application, "go.mod"))
	if err != nil || string(originalManifest) != source["app/go.mod"] {
		t.Fatal("original manifest changed")
	}
}
