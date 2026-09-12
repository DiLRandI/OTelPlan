package otelc

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/mod/modfile"
)

func TestRuntimeModule(t *testing.T) {
	files, err := renderRuntimeModule("example.com/generated")
	if err != nil {
		t.Fatal(err)
	}
	module, err := modfile.Parse("go.mod", files[0].Data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if module.Module.Mod.Path != "example.com/generated" || module.Go.Version != "1.25.0" {
		t.Fatal("incorrect module identity")
	}
	for _, requirement := range module.Require {
		if requirement.Mod.Path == "go.opentelemetry.io/otel/sdk" {
			t.Fatal("runtime installs application SDK")
		}
	}
	root := t.TempDir()
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(root, file.Path), file.Data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	source := []byte("package runtimeprobe\nimport (\n_ \"go.opentelemetry.io/otel\"\n_ \"go.opentelemetry.io/otel/attribute\"\n_ \"go.opentelemetry.io/otel/codes\"\n_ \"go.opentelemetry.io/otel/trace\"\n_ \"go.opentelemetry.io/otelc/pkg/hook\"\n)\n")
	if err := os.WriteFile(filepath.Join(root, "runtime.go"), source, 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "go", "test", "-mod=readonly", "./...")
	command.Dir = root
	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("runtime module does not compile: %v\n%s", err, output)
	}
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(root, file.Path))
		if err != nil || !bytes.Equal(data, file.Data) {
			t.Fatal("runtime build modified pinned module state")
		}
	}
	files[1].Data[0] ^= 1
	repeated, err := renderRuntimeModule("example.com/generated")
	if err != nil || bytes.Equal(files[1].Data, repeated[1].Data) {
		t.Fatal("caller mutated embedded checksums")
	}
}
