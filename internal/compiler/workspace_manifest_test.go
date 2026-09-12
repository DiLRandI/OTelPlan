package compiler

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/mod/modfile"
)

func TestRelocateWorkspaceManifest(t *testing.T) {
	original, copied, runtime := t.TempDir(), t.TempDir(), t.TempDir()
	data := []byte("go 1.25.0\ntoolchain go1.27.0\nuse ./app\nreplace example.com/dep => ./dep\nreplace example.com/other => example.com/fork v1.2.0\n")
	before := append([]byte(nil), data...)
	mapping := map[string]string{filepath.Join(original, "app"): filepath.Join(copied, "app"), filepath.Join(original, "dep"): filepath.Join(copied, "dep")}
	output, err := RelocateWorkspaceManifest(data, original, runtime, mapping)
	if err != nil {
		t.Fatal(err)
	}
	work, err := modfile.ParseWork("go.work", output, nil)
	if err != nil {
		t.Fatal(err)
	}
	if work.Go.Version != "1.25.0" || work.Toolchain.Name != "go1.27.0" || len(work.Use) != 2 {
		t.Fatal("workspace directives changed")
	}
	foundRuntime := false
	for _, use := range work.Use {
		if use.Path == runtime {
			foundRuntime = true
		} else if use.Path != filepath.Join(copied, "app") {
			t.Fatal("application path not relocated")
		}
	}
	if !foundRuntime || work.Replace[0].New.Path != filepath.Join(copied, "dep") || work.Replace[1].New.Version != "v1.2.0" {
		t.Fatal("workspace semantics changed")
	}
	repeated, err := RelocateWorkspaceManifest(data, original, runtime, mapping)
	if err != nil || !bytes.Equal(output, repeated) || !bytes.Equal(data, before) {
		t.Fatal("rendering changed caller or output")
	}
	if output, err := RelocateWorkspaceManifest(data, original, runtime, nil); err == nil || output != nil {
		t.Fatal("accepted missing copy mappings")
	}
	if output, err := RelocateWorkspaceManifest(data, original, filepath.Join(copied, "app"), mapping); err == nil || output != nil {
		t.Fatal("accepted overlapping runtime")
	}
}

func TestBuildRelocatedWorkspace(t *testing.T) {
	original, staging := t.TempDir(), t.TempDir()
	files := map[string]string{
		"app/go.mod":     "module example.com/app\n\ngo 1.25.0\nrequire example.com/dep v1.0.0\n",
		"app/main.go":    "package main\nimport (\"fmt\";\"example.com/dep\")\nfunc main(){fmt.Print(dep.Value)}\n",
		"dep/go.mod":     "module example.com/dep\n\ngo 1.25.0\n",
		"dep/dep.go":     "package dep\nconst Value = \"workspace copy\"\n",
		"runtime/go.mod": "module example.com/runtime\n\ngo 1.25.0\n",
	}
	for path, data := range files {
		filename := filepath.Join(original, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	mapping := map[string]string{}
	for _, name := range []string{"app", "dep", "runtime"} {
		source := filepath.Join(original, name)
		copied, err := CopySourceTree(t.Context(), source, staging)
		if err != nil {
			t.Fatal(err)
		}
		mapping[source] = copied
	}
	input := []byte("go 1.25.0\nuse ./app\nreplace example.com/dep => ./dep\n")
	rendered, err := RelocateWorkspaceManifest(input, original, mapping[filepath.Join(original, "runtime")], mapping)
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(staging, "go.work")
	if err := os.WriteFile(workspace, rendered, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(original, "dep"), filepath.Join(original, "old-dep")); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "go", "run", "-mod=readonly", ".")
	command.Dir = mapping[filepath.Join(original, "app")]
	command.Env = append(os.Environ(), "GOWORK="+workspace, "GOFLAGS=", "GOPROXY=off")
	output, err := command.CombinedOutput()
	if err != nil || string(output) != "workspace copy" {
		t.Fatalf("relocated workspace build failed: %v\n%s", err, output)
	}
	unchanged, err := os.ReadFile(workspace)
	if err != nil || !bytes.Equal(unchanged, rendered) {
		t.Fatal("Go changed workspace manifest")
	}
}
