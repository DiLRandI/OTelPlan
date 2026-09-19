package cli

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
)

func TestBuildCLIWithPinnedBackend(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required")
	}
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	fixture, err := filepath.Abs("../backend/otelc/testdata/accessors")
	if err != nil {
		t.Fatal(err)
	}
	root, err := compiler.CopySourceTree(t.Context(), fixture, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	policy := []byte("apiVersion: otelplan.io/v1alpha1\nkind: InstrumentationPlan\nbackend: {name: otelc, version: v1.1.0}\nproject: {packages: [./ops]}\nrules:\n- id: operation\n  match:\n    methods: [Handle]\n")
	if err := os.WriteFile(filepath.Join(root, "otelplan.yaml"), policy, 0600); err != nil {
		t.Fatal(err)
	}
	original := map[string][]byte{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		original[path] = data
		return err
	}); err != nil {
		t.Fatal(err)
	}
	cli := filepath.Join(t.TempDir(), "otelplan")
	command := exec.Command("go", "build", "-o", cli, "../../cmd/otelplan")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v %s", err, output)
	}
	command = exec.Command(cli, "build", "--root", root, "--format=json", "--offline", "--", "-race", "-trimpath", "-buildvcs=false", "-o", "bin/probe", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build command: %v %s", err, output)
	}
	var reply struct {
		OK   bool         `json:"ok"`
		Data buildSummary `json:"data"`
	}
	if err := json.Unmarshal(output, &reply); err != nil || !reply.OK || reply.Data.Digest == "" {
		t.Fatalf("bad build JSON: %s", output)
	}
	output, err = exec.Command(reply.Data.Path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var traces struct{ Spans []struct{ Name string } }
	if err := json.Unmarshal(output, &traces); err != nil || len(traces.Spans) != 5 {
		t.Fatalf("missing instrumented spans: %s", output)
	}
	var guarded, guardErr bytes.Buffer
	if exit := Run([]string{"build", "--root", root, "--format=json", "--", "-o", "go.mod", "."}, &guarded, &guardErr); exit != 2 {
		t.Fatalf("source output exit=%d: %s", exit, &guarded)
	}
	for path, want := range original {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("changed source file %s", path)
		}
	}
}

func TestBuildCLIUsage(t *testing.T) {
	for _, args := range [][]string{{"build", "--", "-o"}, {"build", "--", "-overlay=secret"}, {"build", "--check"}, {"build", "--clean"}} {
		var out, errout bytes.Buffer
		args = append([]string{"--format=json"}, args...)
		if code := Run(args, &out, &errout); code != 2 {
			t.Fatalf("usage exit=%d: %s", code, &out)
		}
		var reply response
		if err := json.Unmarshal(out.Bytes(), &reply); err != nil || reply.OK {
			t.Fatalf("invalid JSON: %s", &out)
		}
	}
}

func TestBuildCLILibraryWithoutOutput(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required")
	}
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root, original := cliFixture(t)
	var out, errout bytes.Buffer
	if exit := Run([]string{"build", "--root", root, "--offline", "--format=json", "--", "-buildvcs=false", "."}, &out, &errout); exit != 0 {
		t.Fatalf("library build exit=%d: %s %s", exit, &out, &errout)
	}
	var reply struct {
		OK   bool         `json:"ok"`
		Data buildSummary `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &reply); err != nil || !reply.OK || reply.Data.Path != "" {
		t.Fatalf("unexpected library output: %s", &out)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != len(original) {
		t.Fatal("library build wrote project files")
	}
}
