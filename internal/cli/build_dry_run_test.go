package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

type buildDryRunReply struct {
	OK   bool `json:"ok"`
	Data struct {
		DryRun  bool              `json:"dryRun"`
		Outputs []string          `json:"outputs"`
		Backend model.LockBackend `json:"backend"`
	} `json:"data"`
}

func TestBuildDryRunPreflightsWithoutExecutingOrPublishing(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required for pinned backend integration")
	}

	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	testenv.PrepareOffline(t, "../..", filepath.Join("testdata", "architectures", "common"))
	root := buildDryRunFixture(t)
	warmCache := filepath.Join(root, "analysis-cache")
	warmBuildCache(t, root, warmCache)
	original := testenv.SnapshotFiles(t, root)
	reply, output, args := runInitialBuildDryRun(t, root)
	assertInitialBuildDryRun(t, root, reply)

	assertBuildDryRunRepeated(t, args, output)
	assertBuildDryRunText(t, root, args)
	assertBuildDryRunDefaultOutput(t, root, warmCache)
	assertBuildDryRunDirectory(t, root)
	assertBuildDryRunLibrary(t, root)
	assertBuildDryRunMissingCache(t, root)
	assertBuildNormalFailure(t, root)
	assertBuildDryRunOutputSafety(t, root)
	assertBuildDryRunCancellation(t, root)
	testenv.AssertFilesUnchanged(t, original, testenv.SnapshotFiles(t, root))
}

func warmBuildCache(t *testing.T, root, cache string) {
	t.Helper()

	args := []string{"inspect", "--root", root, "--offline", "--format=json", "--cache-dir", cache}

	var stdout, stderr bytes.Buffer
	if code := cli.Run(t.Context(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("warm cache inspect exit=%d: %s %s", code, &stdout, &stderr)
	}

	entries, err := os.ReadDir(cache)
	if err != nil || len(entries) == 0 {
		t.Fatalf("warm cache is empty: %v", err)
	}
}

func runInitialBuildDryRun(t *testing.T, root string) (buildDryRunReply, []byte, []string) {
	t.Helper()

	cache := filepath.Join(root, "analysis-cache")
	args := []string{"build", "--root", root, "--dry-run", "--offline", "--format=json",
		"--cache-dir", cache, "--", "-buildvcs=false", "-o", "probe", "."}

	var stdout, stderr bytes.Buffer

	if code := cli.Run(t.Context(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("build dry-run exit=%d: %s %s", code, &stdout, &stderr)
	}

	var reply buildDryRunReply

	err := json.Unmarshal(stdout.Bytes(), &reply)
	if err != nil {
		t.Fatalf("decode build dry-run response: %v", err)
	}

	return reply, stdout.Bytes(), args
}

func assertInitialBuildDryRun(t *testing.T, root string, reply buildDryRunReply) {
	t.Helper()

	wantOutput := filepath.Join(root, "probe")
	if !reply.OK || !reply.Data.DryRun || reply.Data.Backend.Digest == "" || len(reply.Data.Outputs) != 1 ||
		reply.Data.Outputs[0] != wantOutput {
		t.Fatalf("invalid build dry-run response: %+v", reply)
	}

	_, err := os.Stat(wantOutput)
	if !os.IsNotExist(err) {
		t.Fatalf("dry-run published binary: %v", err)
	}
}

func assertBuildDryRunRepeated(t *testing.T, args []string, want []byte) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	if code := cli.Run(t.Context(), args, &stdout, &stderr); code != 0 || stdout.String() != string(want) {
		t.Fatalf("repeated build dry-run changed response: exit=%d stderr=%s output=%s", code, &stderr, &stdout)
	}
}

func assertBuildDryRunText(t *testing.T, root string, args []string) {
	t.Helper()

	textArgs := append([]string{}, args...)
	for index, arg := range textArgs {
		if arg == "--format=json" {
			textArgs[index] = "--format=text"
		}
	}

	var stdout, stderr bytes.Buffer

	if code := cli.Run(t.Context(), textArgs, &stdout, &stderr); code != 0 {
		t.Fatalf("text build dry-run exit=%d: %s %s", code, &stdout, &stderr)
	}

	containsPreview := strings.Contains(stdout.String(), "would build")

	containsOutput := strings.Contains(stdout.String(), filepath.Join(root, "probe"))
	if !containsPreview || !containsOutput {
		t.Fatalf("text build dry-run omitted planned output: %s", stdout.String())
	}
}

func assertBuildDryRunDirectory(t *testing.T, root string) {
	t.Helper()

	args := []string{"build", "--root", root, "--dry-run", "--offline", "--format=json", "--",
		"-buildvcs=false", "-o", "binaries/", ".", "./cmd/worker"}

	var stdout, stderr bytes.Buffer
	if code := cli.Run(t.Context(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("directory build dry-run exit=%d: %s %s", code, &stdout, &stderr)
	}

	var reply buildDryRunReply

	err := json.Unmarshal(stdout.Bytes(), &reply)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{filepath.Join(root, "binaries", "dryrun"), filepath.Join(root, "binaries", "worker")}
	if !reply.Data.DryRun || !slices.Equal(reply.Data.Outputs, want) {
		t.Fatalf("directory preview outputs=%v, want %v", reply.Data.Outputs, want)
	}
}

func assertBuildDryRunLibrary(t *testing.T, root string) {
	t.Helper()

	args := []string{"build", "--root", root, "--dry-run", "--offline", "--format=json", "--", "-buildvcs=false", "./ops"}

	var stdout, stderr bytes.Buffer
	if code := cli.Run(t.Context(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("library build dry-run exit=%d: %s %s", code, &stdout, &stderr)
	}

	var reply buildDryRunReply

	err := json.Unmarshal(stdout.Bytes(), &reply)
	if err != nil {
		t.Fatal(err)
	}

	if !reply.Data.DryRun || len(reply.Data.Outputs) != 0 {
		t.Fatalf("library preview outputs=%v", reply.Data.Outputs)
	}
}

func assertBuildDryRunDefaultOutput(t *testing.T, root, cache string) {
	t.Helper()

	args := []string{"build", "--root", root, "--dry-run", "--offline", "--format=json",
		"--cache-dir", cache, "--", "-buildvcs=false", "."}
	output := runBuildDryRun(t, args)

	want := filepath.Join(root, "dryrun")
	if !slices.Equal(output.Data.Outputs, []string{want}) {
		t.Fatalf("default preview outputs=%v, want %v", output.Data.Outputs, []string{want})
	}
}

func assertBuildDryRunMissingCache(t *testing.T, root string) {
	t.Helper()

	cache := filepath.Join(root, "missing-cache")
	args := []string{"build", "--root", root, "--dry-run", "--offline", "--format=json",
		"--cache-dir", cache, "--", "-buildvcs=false", "-o", "missing-probe", "."}
	runBuildDryRun(t, args)

	_, err := os.Stat(cache)
	if !os.IsNotExist(err) {
		t.Fatalf("missing dry-run cache was created: %v", err)
	}
}

func runBuildDryRun(t *testing.T, args []string) buildDryRunReply {
	t.Helper()

	var stdout, stderr bytes.Buffer
	if code := cli.Run(t.Context(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("build dry-run exit=%d: %s %s", code, &stdout, &stderr)
	}

	var reply buildDryRunReply

	err := json.Unmarshal(stdout.Bytes(), &reply)
	if err != nil {
		t.Fatal(err)
	}

	return reply
}

func assertBuildNormalFailure(t *testing.T, root string) {
	t.Helper()

	args := []string{"build", "--root", root, "--offline", "--format=json", "--",
		"-buildvcs=false", "-o", "normal-probe", "."}

	var stdout, stderr bytes.Buffer
	if code := cli.Run(t.Context(), args, &stdout, &stderr); code != 8 {
		t.Fatalf("normal build exit=%d, want 8: %s %s", code, &stdout, &stderr)
	}

	_, err := os.Stat(filepath.Join(root, "normal-probe"))
	if !os.IsNotExist(err) {
		t.Fatalf("failed normal build published binary: %v", err)
	}
}

func assertBuildDryRunOutputSafety(t *testing.T, root string) {
	t.Helper()

	assertExistingBuildOutputPreserved(t, root)
	assertInvalidBuildDestinations(t, root)
}

func assertExistingBuildOutputPreserved(t *testing.T, root string) {
	t.Helper()

	existing := filepath.Join(root, "existing-probe")

	err := os.WriteFile(existing, []byte("keep"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	args := []string{"build", "--root", root, "--dry-run", "--offline", "--format=json", "--",
		"-buildvcs=false", "-o", existing, "."}

	var stdout, stderr bytes.Buffer
	if code := cli.Run(t.Context(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("existing-output dry-run exit=%d: %s %s", code, &stdout, &stderr)
	}

	directory, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}

	data, err := directory.ReadFile(filepath.Base(existing))
	_ = directory.Close()

	if err != nil || string(data) != "keep" {
		t.Fatal("dry-run changed existing output")
	}

	err = os.Remove(existing)
	if err != nil {
		t.Fatal(err)
	}
}

func assertInvalidBuildDestinations(t *testing.T, root string) {
	t.Helper()

	invalid := []string{"go.mod", filepath.Join("metadata-file", "probe")}

	err := os.WriteFile(filepath.Join(root, "metadata-file"), []byte("file"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	for _, output := range invalid {
		var invalidOut, invalidErr bytes.Buffer

		args := []string{"build", "--root", root, "--dry-run", "--offline", "--format=json", "--",
			"-buildvcs=false", "-o", output, "."}
		if code := cli.Run(t.Context(), args, &invalidOut, &invalidErr); code == 0 {
			t.Fatalf("invalid dry-run destination %q succeeded", output)
		}
	}

	err = os.Remove(filepath.Join(root, "metadata-file"))
	if err != nil {
		t.Fatal(err)
	}
}

func assertBuildDryRunCancellation(t *testing.T, root string) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var stdout, stderr bytes.Buffer

	args := []string{"build", "--root", root, "--dry-run", "--offline", "--format=json", "--",
		"-buildvcs=false", "."}
	if code := cli.Run(ctx, args, &stdout, &stderr); code != 4 {
		t.Fatalf("canceled build dry-run exit=%d: %s %s", code, &stdout, &stderr)
	}
}

func buildDryRunFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()

	files := map[string]string{
		"go.mod": `module example.com/dryrun

go 1.27
`,
		"main.go": `package main

import (
	"context"
	"example.com/dryrun/ops"
)

func main() {
	ops.Run(context.Background())
	undefinedFunction()
}
`,
		"ops/run.go": `package ops

import "context"

func Run(context.Context) error { return nil }
`,
		"cmd/worker/main.go": `package main

import (
	"context"
	"example.com/dryrun/ops"
)

func main() { _ = ops.Run(context.Background()) }
`,
		"otelplan.yaml": `apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
project: {packages: [./ops]}
rules:
- id: operation
  match:
    functions: [Run]
`,
	}
	for name, data := range files {
		err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o700)
		if err != nil {
			t.Fatal(err)
		}

		err = os.WriteFile(filepath.Join(root, name), []byte(data), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	return root
}
