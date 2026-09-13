package compiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/modfile"
)

func TestPrepareWorkspaceAlternateModfile(t *testing.T) {
	source := t.TempDir()
	alternateDir := t.TempDir()
	original := "module example.com/original\n\ngo 1.25.0\n"
	alternate := "module example.com/alternate\n\ngo 1.25.0\nreplace example.com/dependency => ./dependency\n"
	write := func(path, data string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dependency := filepath.Join(source, "dependency")
	if err := os.Mkdir(dependency, 0700); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(dependency, "go.mod"), "module example.com/dependency\n\ngo 1.25.0\n")
	write(filepath.Join(source, "go.mod"), original)
	write(filepath.Join(source, "go.sum"), "original checksums\n")
	alternatePath := filepath.Join(alternateDir, "build.mod")
	write(alternatePath, alternate)
	runtimeDir := t.TempDir()
	runtimeModule := []byte("module example.com/runtime\n\ngo 1.25.0\n")
	write(filepath.Join(runtimeDir, "go.mod"), string(runtimeModule))
	runtime := model.Artifacts{Dir: runtimeDir, Files: []model.ArtifactFile{{Path: "go.mod", Digest: artifactDigest(runtimeModule)}}}
	request := WorkspaceRequest{OriginalWorkspaceDir: source, Workspace: []byte("go 1.25.0\nuse .\n"), Runtime: runtime, Parent: t.TempDir(), AlternateModFiles: map[string]string{source: alternatePath}}
	for _, sums := range []string{"", "alternate checksums\n"} {
		if sums != "" {
			write(filepath.Join(alternateDir, "build.sum"), sums)
		}
		prepared, err := PrepareWorkspace(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		copiedManifest, err := os.ReadFile(filepath.Join(prepared.Relocations[source], "go.mod"))
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := modfile.Parse("go.mod", copiedManifest, nil)
		if err != nil || len(parsed.Replace) != 1 || parsed.Replace[0].New.Path != prepared.Relocations[dependency] {
			t.Fatal("alternate replacement did not resolve relative to original module")
		}
		command := exec.CommandContext(t.Context(), "go", "list", "-m")
		command.Dir = prepared.Relocations[source]
		command.Env = append(os.Environ(), "GOFLAGS=", "GOWORK=off", "GOPROXY=off")
		output, err := command.Output()
		if err != nil || string(output) != "example.com/alternate\n" {
			t.Fatalf("alternate manifest not used: %v %s", err, output)
		}
		copiedSums, err := os.ReadFile(filepath.Join(prepared.Relocations[source], "go.sum"))
		if sums == "" {
			if !os.IsNotExist(err) {
				t.Fatal("original checksums survived alternate selection")
			}
		} else if err != nil || string(copiedSums) != sums {
			t.Fatal("alternate checksums not copied")
		}
	}
	for path, want := range map[string]string{filepath.Join(source, "go.mod"): original, filepath.Join(source, "go.sum"): "original checksums\n", alternatePath: alternate} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatal("original module files changed")
		}
	}
	malformed := filepath.Join(alternateDir, "malformed.mod")
	write(malformed, "not a module manifest\n")
	for _, selection := range []map[string]string{
		{t.TempDir(): alternatePath},
		{source: "relative.mod"},
		{source: filepath.Join(alternateDir, "missing.mod")},
		{source: malformed},
		{source: filepath.Join(alternateDir, "wrong.txt")},
	} {
		request.AlternateModFiles = selection
		request.Parent = t.TempDir()
		if _, err := PrepareWorkspace(t.Context(), request); err == nil {
			t.Fatal("accepted invalid alternate module selection")
		}
		entries, err := os.ReadDir(request.Parent)
		if err != nil || len(entries) != 0 {
			t.Fatal("invalid selection left output")
		}
	}
}
