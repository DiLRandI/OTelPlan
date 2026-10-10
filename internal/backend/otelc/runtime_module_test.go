package otelc_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/modfile"
)

func TestRuntimeModule(t *testing.T) {
	t.Parallel()

	firstFiles := renderRuntimeBundle(t)
	moduleFiles := runtimeModuleFiles(t, firstFiles)

	checkRuntimeModuleIdentity(t, moduleFiles["go.mod"])

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := root.Close()
		if err != nil {
			t.Errorf("close generated module root: %v", err)
		}
	})

	for path, data := range moduleFiles {
		err := root.WriteFile(path, data, 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	err = root.WriteFile("runtime.go", runtimeProbeSource(), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = compileRuntimeProbe(t, root.Name())
	if err != nil {
		t.Fatal(err)
	}

	assertRootFilesUnchanged(t, root, moduleFiles)

	checksumBaseline := bytes.Clone(moduleFiles["go.sum"])
	mutatedChecksums := moduleFiles["go.sum"]
	mutatedChecksums[0] ^= 1
	secondFiles := runtimeModuleFiles(t, renderRuntimeBundle(t))

	if bytes.Equal(mutatedChecksums, secondFiles["go.sum"]) {
		t.Fatal("caller mutated embedded checksums")
	}

	if !bytes.Equal(checksumBaseline, secondFiles["go.sum"]) {
		t.Fatal("caller mutation changed embedded checksum state")
	}
}

func renderRuntimeBundle(t *testing.T) []otelc.GeneratedFile {
	t.Helper()

	backend, err := otelc.Identity(otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	var plan model.ResolvedPlan

	code := new(model.CodeModel)

	files, err := otelc.RenderBundle(backend, "test", code, plan, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}

	return files
}

func runtimeModuleFiles(t *testing.T, files []otelc.GeneratedFile) map[string][]byte {
	t.Helper()

	moduleFiles := make(map[string][]byte, 2)

	for _, file := range files {
		if file.Path == "go.mod" || file.Path == "go.sum" {
			moduleFiles[file.Path] = file.Data
		}
	}

	for _, path := range []string{"go.mod", "go.sum"} {
		if len(moduleFiles[path]) == 0 {
			t.Fatalf("generated bundle is missing %s", path)
		}
	}

	return moduleFiles
}

func checkRuntimeModuleIdentity(t *testing.T, data []byte) {
	t.Helper()

	module, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		t.Fatalf("parse generated go.mod: %v", err)
	}

	if module.Module.Mod.Path != "example.com/generated" || module.Go.Version != "1.25.0" {
		t.Fatal("incorrect generated module identity or Go version")
	}

	for _, requirement := range module.Require {
		if requirement.Mod.Path == "go.opentelemetry.io/otel/sdk" {
			t.Fatal("runtime module installs application SDK")
		}
	}
}

func runtimeProbeSource() []byte {
	return []byte(`package runtimeprobe

import (
	_ "go.opentelemetry.io/otel"
	_ "go.opentelemetry.io/otel/attribute"
	_ "go.opentelemetry.io/otel/codes"
	_ "go.opentelemetry.io/otel/trace"
	_ "go.opentelemetry.io/otelc/pkg/hook"
)
`)
}

func compileRuntimeProbe(t *testing.T, rootPath string) error {
	t.Helper()

	command := exec.CommandContext(t.Context(), "go", "test", "-mod=readonly", "./...")
	command.Dir = rootPath

	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")

	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("runtime module does not compile: %w\n%s", err, output)
	}

	return nil
}

func assertRootFilesUnchanged(t *testing.T, root *os.Root, want map[string][]byte) {
	t.Helper()

	for path, expected := range want {
		actual, err := root.ReadFile(path)
		if err != nil {
			t.Fatalf("read generated %s after compilation: %v", path, err)
		}

		if !bytes.Equal(actual, expected) {
			t.Fatalf("runtime build modified pinned %s state", path)
		}
	}
}
