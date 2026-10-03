package compiler_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
)

func TestRelocateBuildArguments(t *testing.T) {
	t.Parallel()

	source, parent := t.TempDir(), t.TempDir()
	app, other := filepath.Join(source, "app"), filepath.Join(source, "other")

	copiedApp, copiedOther := filepath.Join(parent, "app"), filepath.Join(parent, "other")
	for _, dir := range []string{copiedApp, copiedOther, filepath.Join(copiedApp, "cmd")} {
		err := os.MkdirAll(dir, 0o700)
		if err != nil {
			t.Fatal(err)
		}
	}

	workspace := new(compiler.PreparedWorkspace)
	workspace.Dir, workspace.Relocations = parent, map[string]string{app: copiedApp, other: copiedOther}
	args := []string{
		"-tags", "a,b", "-p=2", ".", "./cmd/...", "../other", filepath.Join(app, "main.go"), "helper.go",
		"example.com/app/...", "./cmd/.../nested",
	}
	before := slices.Clone(args)
	want := []string{
		"-tags", "a,b", "-p=2", copiedApp, filepath.Join(copiedApp, "cmd", "..."), copiedOther,
		filepath.Join(copiedApp, "main.go"), filepath.Join(copiedApp, "helper.go"), "example.com/app/...",
		filepath.Join(copiedApp, "cmd", "...", "nested"),
	}

	got, err := workspace.RelocateBuildArguments(args, app)
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("relocated arguments = %v, %v; want %v", got, err, want)
	}

	if !slices.Equal(args, before) {
		t.Fatal("caller arguments changed")
	}

	for _, arg := range []string{"../../outside", filepath.Join(source, "unprepared"), "../missing/..."} {
		_, err := workspace.RelocateBuildArguments([]string{arg}, app)
		if err == nil {
			t.Fatal("accepted unprepared package path")
		}
	}
}

func TestBuildPathRelocationErrorsShareIdentity(t *testing.T) {
	t.Parallel()

	workspace := new(compiler.PreparedWorkspace)
	_, first := workspace.RelocateBuildArguments(nil, ".")

	_, second := workspace.RelocateBuildArguments(nil, "relative")
	if first == nil || !errors.Is(second, first) || first.Error() != "original build directory must be absolute" {
		t.Fatal("invalid origin lost its stable cause or meaningful error")
	}
}

func TestBuildPathRelocationPreservesFlagValues(t *testing.T) {
	t.Parallel()

	source, copied := t.TempDir(), t.TempDir()
	workspace := new(compiler.PreparedWorkspace)
	workspace.Dir, workspace.Relocations = copied, map[string]string{source: copied}
	args := []string{"-o", "output.go", "-tags", "tag.go", "-p", "2", "-mod", "readonly", "input.go"}
	original := slices.Clone(args)

	relocated, err := workspace.RelocateBuildArguments(args, source)
	if err != nil {
		t.Fatal(err)
	}

	expected := slices.Clone(args)

	expected[len(expected)-1] = filepath.Join(copied, "input.go")
	if !slices.Equal(relocated, expected) || !slices.Equal(args, original) {
		t.Fatal("path-like flag values were relocated or caller input changed")
	}
}

func TestBuildPathRelocationRetainsWorkspaceMapping(t *testing.T) {
	t.Parallel()

	source, copied := t.TempDir(), t.TempDir()
	workspace := new(compiler.PreparedWorkspace)
	workspace.Dir, workspace.Relocations = copied, map[string]string{source: copied}

	_, err := workspace.RelocateBuildArguments([]string{"./..."}, source)
	if err != nil || workspace.Relocations[source] != copied {
		t.Fatal("relocation changed its verified directory mapping", err)
	}
}

func TestBuildPathRelocationRejectsFileAsDirectory(t *testing.T) {
	t.Parallel()

	source, copied := t.TempDir(), t.TempDir()

	root, err := os.OpenRoot(copied)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = root.WriteFile("looks.go", []byte("package example\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	workspace := new(compiler.PreparedWorkspace)
	workspace.Dir, workspace.Relocations = copied, map[string]string{source: copied}

	relocated, err := workspace.RelocateBuildArguments([]string{"./looks.go/."}, source)
	if err == nil || relocated != nil {
		t.Fatal("package directory syntax was incorrectly accepted as a Go filename")
	}
}
