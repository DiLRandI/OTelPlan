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

	second := filepath.Join(root, "cmd", "second")
	if err := os.MkdirAll(second, 0700); err != nil {
		t.Fatal(err)
	}
	mainSource, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "main.go"), mainSource, 0600); err != nil {
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

	for _, existing := range []bool{false, true} {
		destination := filepath.Join(t.TempDir(), "binaries")
		argument := destination + string(os.PathSeparator)
		if existing {
			if err := os.Mkdir(destination, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(destination, "keep"), []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			argument = destination
		}
		var out, errout bytes.Buffer
		if exit := Run([]string{"build", "--root", root, "--format=json", "--offline", "--", "-buildvcs=false", "-o", argument, ".", "./cmd/second"}, &out, &errout); exit != 0 {
			t.Fatalf("directory build exit=%d: %s %s", exit, &out, &errout)
		}
		var directoryReply struct {
			OK   bool `json:"ok"`
			Data struct {
				Files []struct{ Path, Digest string } `json:"files"`
			} `json:"data"`
		}
		if err := json.Unmarshal(out.Bytes(), &directoryReply); err != nil || !directoryReply.OK || len(directoryReply.Data.Files) != 2 {
			t.Fatalf("invalid directory response: %s", &out)
		}
		for _, file := range directoryReply.Data.Files {
			if filepath.Dir(file.Path) != destination || file.Digest == "" {
				t.Fatalf("invalid published file: %+v", file)
			}
			output, err := exec.Command(file.Path).Output()
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(output, &traces); err != nil || len(traces.Spans) != 5 {
				t.Fatalf("missing instrumented spans: %s", output)
			}
		}

		if existing {
			first, last := directoryReply.Data.Files[0].Path, directoryReply.Data.Files[1].Path
			previous := filepath.Join(destination, "previous")
			if err := os.WriteFile(previous, []byte("previous"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(previous, first); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(last); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(last, 0700); err != nil {
				t.Fatal(err)
			}
			out.Reset()
			errout.Reset()
			if exit := Run([]string{"build", "--root", root, "--format=json", "--offline", "--", "-buildvcs=false", "-o", argument, ".", "./cmd/second"}, &out, &errout); exit != 1 {
				t.Fatalf("invalid destination exit=%d: %s", exit, &out)
			}
			if data, err := os.ReadFile(first); err != nil || string(data) != "previous" {
				t.Fatal("invalid destination partially replaced output")
			}
		}
		if existing {
			data, err := os.ReadFile(filepath.Join(destination, "keep"))
			if err != nil || string(data) != "keep" {
				t.Fatal("changed unrelated output directory file")
			}
		}
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

func TestBuildCLIFromWorkspaceRoot(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required")
	}
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	fixture, err := filepath.Abs("../backend/otelc/testdata/accessors")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	copied, err := compiler.CopySourceTree(t.Context(), fixture, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(copied, filepath.Join(root, "app")); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"go.work":       "go 1.27.0\nuse ./app\n",
		"otelplan.yaml": "apiVersion: otelplan.io/v1alpha1\nkind: InstrumentationPlan\nbackend: {name: otelc, version: v1.1.0}\nproject: {packages: [./app/ops]}\nrules:\n- id: operation\n  match:\n    methods: [Handle]\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
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
	var out, errout bytes.Buffer
	if exit := Run([]string{"build", "--root", root, "--offline", "--format=json", "--", "-buildvcs=false", "./app"}, &out, &errout); exit != 0 {
		t.Fatalf("workspace build exit=%d: %s %s", exit, &out, &errout)
	}
	var reply struct {
		OK   bool         `json:"ok"`
		Data buildSummary `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &reply); err != nil || !reply.OK || reply.Data.Path == "" || reply.Data.Digest == "" {
		t.Fatalf("invalid build response: %s", &out)
	}
	if filepath.Dir(reply.Data.Path) != root {
		t.Fatal("output not published in original working directory")
	}
	output, err := exec.Command(reply.Data.Path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var traces struct{ Spans []struct{ Name string } }
	if err := json.Unmarshal(output, &traces); err != nil || len(traces.Spans) != 5 {
		t.Fatalf("missing instrumented spans: %s", output)
	}
	out.Reset()
	errout.Reset()
	if exit := Run([]string{"build", "--root", root, "--offline", "--format=json", "--", "-buildvcs=false"}, &out, &errout); exit == 0 {
		t.Fatal("workspace build without target silently selected a module")
	}
	for path, want := range original {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("changed source file %s", path)
		}
	}
}

func TestBuildSummaryText(t *testing.T) {
	for _, tc := range []struct {
		data buildSummary
		want string
	}{
		{buildSummary{}, "build succeeded; no executable output\n"},
		{buildSummary{Path: "app", Digest: "one"}, "built app one\n"},
		{buildSummary{Files: []buildFile{{Path: "bin/first", Digest: "one"}, {Path: "bin/second", Digest: "two"}}}, "built bin/first one\nbuilt bin/second two\n"},
	} {
		var out bytes.Buffer
		if err := emit(&out, options{format: "text"}, response{OK: true, Data: tc.data}); err != nil || out.String() != tc.want {
			t.Fatalf("output=%q, %v; want %q", out.String(), err, tc.want)
		}
	}
}
