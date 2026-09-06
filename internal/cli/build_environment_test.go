package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockChecksPreserveExplicitModModeInputs(t *testing.T) {
	root, files := cliFixture(t)
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOWORK", "off")
	files["go.mod"] += "\nreplace example.com/local => ./local\n"
	files["dependency.go"] = "package app\nimport _ \"example.com/local\"\n"
	files["local/go.mod"] = "module example.com/local\n\ngo 1.27\n"
	files["local/local.go"] = "package local\n"
	for name, contents := range files {
		filename := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	invoke(t, root, 0, "lock")
	invoke(t, root, 0, "lock", "--check")
	invoke(t, root, 0, "diff", "--check")
	for name, contents := range files {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(got) != contents {
			t.Fatalf("analysis modified %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "go.sum")); !os.IsNotExist(err) {
		t.Fatal("analysis created a project go.sum")
	}
}

func TestLockChecksDetectAmbientBuildTags(t *testing.T) {
	root, _ := cliFixture(t)
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-tags=first")
	invoke(t, root, 0, "lock")
	invoke(t, root, 0, "lock", "--check")
	t.Setenv("GOFLAGS", "-tags=second")
	invoke(t, root, 6, "lock", "--check")
}
