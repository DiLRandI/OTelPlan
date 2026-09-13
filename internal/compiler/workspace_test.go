package compiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestPrepareWorkspace(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "go.mod"), []byte("module example.com/app\n\ngo 1.25.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	app := []byte("package main\nimport \"fmt\"\nfunc main(){fmt.Print(\"prepared\")}\n")
	if err := os.WriteFile(filepath.Join(source, "main.go"), app, 0600); err != nil {
		t.Fatal(err)
	}
	backend, _ := otelc.Identity(otelc.SupportedVersion)
	files, err := otelc.RenderBundle(backend, "test", &model.CodeModel{}, model.ResolvedPlan{}, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := StageArtifacts(t.TempDir(), files)
	if err != nil {
		t.Fatal(err)
	}
	request := WorkspaceRequest{SourceDirs: []string{source}, OriginalWorkspaceDir: source, Workspace: []byte("go 1.25.0\nuse .\n"), Runtime: runtime, Parent: t.TempDir()}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	request.Parent, err = filepath.Rel(cwd, request.Parent)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareWorkspace(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(prepared.WorkspaceFile) {
		t.Fatal("workspace path must be absolute")
	}
	if err := VerifyArtifacts(prepared.Runtime); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "go", "run", "-mod=readonly", ".")
	command.Dir = prepared.Relocations[source]
	command.Env = append(os.Environ(), "GOWORK="+prepared.WorkspaceFile, "GOFLAGS=", "GOPROXY=off")
	output, err := command.CombinedOutput()
	if err != nil || string(output) != "prepared" {
		t.Fatalf("prepared build failed: %v\n%s", err, output)
	}
	if err := os.WriteFile(filepath.Join(prepared.Relocations[source], "main.go"), []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(source, "main.go"))
	if err != nil || string(original) != string(app) {
		t.Fatal("original source changed")
	}
	if err := VerifyArtifacts(runtime); err != nil {
		t.Fatal("original runtime changed")
	}
	request.Parent = t.TempDir()
	request.Workspace = []byte("go 1.25.0\nuse ./missing\n")
	if _, err := PrepareWorkspace(t.Context(), request); err == nil {
		t.Fatal("accepted uncopied workspace module")
	}
	entries, err := os.ReadDir(request.Parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("failed preparation left output")
	}
}
