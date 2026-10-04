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

func TestRelocateWorkspaceManifest(t *testing.T) {
	t.Parallel()

	original, copied, runtimeDir := t.TempDir(), t.TempDir(), t.TempDir()
	data := []byte("go 1.25.0\ntoolchain go1.27.0\nuse ./app\nreplace example.com/dep => ./dep\n" +
		"replace example.com/other => example.com/fork v1.2.0\n")
	before := bytes.Clone(data)
	mapping := map[string]string{
		filepath.Join(original, "app"): filepath.Join(copied, "app"),
		filepath.Join(original, "dep"): filepath.Join(copied, "dep"),
	}

	output, err := compiler.RelocateWorkspaceManifest(data, original, runtimeDir, mapping)
	if err != nil {
		t.Fatal(err)
	}

	work, err := modfile.ParseWork("go.work", output, nil)
	if err != nil {
		t.Fatal(err)
	}

	assertWorkspaceDirectivesAndMembers(t, work, copied, runtimeDir)

	if work.Replace[0].New.Path != filepath.Join(copied, "dep") || work.Replace[1].New.Version != "v1.2.0" {
		t.Fatal("workspace replacement semantics changed")
	}

	repeated, err := compiler.RelocateWorkspaceManifest(data, original, runtimeDir, mapping)
	if err != nil || !bytes.Equal(output, repeated) || !bytes.Equal(data, before) {
		t.Fatal("rendering changed caller or output", err)
	}
}

func assertWorkspaceDirectivesAndMembers(t *testing.T, work *modfile.WorkFile, copied, runtimeDir string) {
	t.Helper()

	if work.Go.Version != "1.25.0" || work.Toolchain.Name != "go1.27.0" || len(work.Use) != 2 {
		t.Fatal("workspace directives changed")
	}

	foundRuntime := false

	for _, use := range work.Use {
		if use.Path == runtimeDir {
			foundRuntime = true
		} else if use.Path != filepath.Join(copied, "app") {
			t.Fatal("application path not relocated")
		}
	}

	if !foundRuntime {
		t.Fatal("generated runtime was omitted")
	}
}

func TestWorkspaceRelocationRejectsMissingAndOverlappingCopies(t *testing.T) {
	t.Parallel()

	original, copied, runtimeDir := t.TempDir(), t.TempDir(), t.TempDir()
	input := []byte("go 1.25.0\nuse ./app\n")
	mappings := map[string]string{filepath.Join(original, "app"): filepath.Join(copied, "app")}

	output, err := compiler.RelocateWorkspaceManifest(input, original, runtimeDir, nil)
	if err == nil || output != nil {
		t.Fatal("accepted missing copy mappings")
	}

	output, err = compiler.RelocateWorkspaceManifest(input, original, filepath.Join(copied, "app"), mappings)
	if err == nil || output != nil {
		t.Fatal("accepted overlapping runtime")
	}
}

func TestWorkspaceRelocationRejectsDuplicateCopiedMembers(t *testing.T) {
	t.Parallel()

	original, copied, runtimeDir := t.TempDir(), t.TempDir(), t.TempDir()
	input := []byte("go 1.25.0\nuse (\n./app\n./other\n)\n")
	mappings := map[string]string{filepath.Join(original, "app"): copied, filepath.Join(original, "other"): copied}

	output, err := compiler.RelocateWorkspaceManifest(input, original, runtimeDir, mappings)
	if err == nil || output != nil || err.Error() != "workspace has duplicate copied modules" {
		t.Fatal("accepted duplicate copied members or lost meaningful context", err)
	}
}

func TestWorkspaceRelocationInvalidDirectoriesShareCause(t *testing.T) {
	t.Parallel()

	_, first := compiler.RelocateWorkspaceManifest(nil, "relative", t.TempDir(), nil)

	_, second := compiler.RelocateWorkspaceManifest(nil, "another-relative", t.TempDir(), nil)
	if first == nil || !errors.Is(second, first) || first.Error() != "workspace directories must be absolute" {
		t.Fatal("invalid directories lost their shared cause or meaningful message")
	}
}

func workspaceBuildFixture(t *testing.T) (*os.Root, map[string]string) {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	files := map[string]string{
		"app/go.mod":     "module example.com/app\n\ngo 1.25.0\nrequire example.com/dep v1.0.0\n",
		"app/main.go":    "package main\nimport (\"fmt\";\"example.com/dep\")\nfunc main(){fmt.Print(dep.Value)}\n",
		"dep/go.mod":     "module example.com/dep\n\ngo 1.25.0\n",
		"dep/dep.go":     "package dep\nconst Value = \"workspace copy\"\n",
		"runtime/go.mod": "module example.com/runtime\n\ngo 1.25.0\n",
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

func copyWorkspaceBuildFixture(t *testing.T, original *os.Root, staging string) map[string]string {
	t.Helper()

	mappings := map[string]string{}

	for _, name := range []string{"app", "dep", "runtime"} {
		source := filepath.Join(original.Name(), name)

		copied, err := compiler.CopySourceTree(t.Context(), source, staging)
		if err != nil {
			t.Fatal(err)
		}

		mappings[source] = copied
	}

	return mappings
}

func TestBuildRelocatedWorkspace(t *testing.T) {
	t.Parallel()

	original, _ := workspaceBuildFixture(t)
	staging := t.TempDir()
	mappings := copyWorkspaceBuildFixture(t, original, staging)
	input := []byte("go 1.25.0\nuse ./app\nreplace example.com/dep => ./dep\n")

	rendered, err := compiler.RelocateWorkspaceManifest(input, original.Name(),
		mappings[filepath.Join(original.Name(), "runtime")], mappings)
	if err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(staging)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = root.WriteFile("go.work", rendered, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = original.Rename("dep", "old-dep")
	if err != nil {
		t.Fatal(err)
	}

	command := exec.CommandContext(t.Context(), "go", "run", "-mod=readonly", ".")
	command.Dir = mappings[filepath.Join(original.Name(), "app")]
	command.Env = append(os.Environ(), "GOWORK="+filepath.Join(staging, "go.work"), "GOFLAGS=", "GOPROXY=off")

	output, err := command.CombinedOutput()
	if err != nil || string(output) != "workspace copy" {
		t.Fatalf("relocated workspace build failed: %v\n%s", err, output)
	}

	unchanged, err := root.ReadFile("go.work")
	if err != nil || !bytes.Equal(unchanged, rendered) {
		t.Fatal("Go changed workspace manifest", err)
	}
}
