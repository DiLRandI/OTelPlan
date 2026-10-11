package cli_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestResolutionCommandsPreserveBuildIdentity(t *testing.T) {
	t.Parallel()

	root, directory, files := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n",
		executionPolicy)
	runResolutionCommand(t, root, 0, "lock")
	filename := filepath.Join(root, "otelplan.lock")

	locked, err := lockfile.Parse(readRootFile(t, directory, "otelplan.lock"))
	if err != nil {
		t.Fatal(err)
	}

	locked.Backend.Digest = lockfile.Digest([]byte("verified executable"))

	locked.Artifacts = []model.ArtifactFile{{Path: "rules.yaml", Digest: lockfile.Digest([]byte("generated rules"))}}

	err = lockfile.Write(filename, locked)
	if err != nil {
		t.Fatal(err)
	}

	beforeReadOnly := snapshotRootFiles(t, directory, files, "otelplan.lock")

	for _, args := range [][]string{{"lock", "--check"}, {"diff", "--check"}, {"validate"}} {
		runResolutionCommand(t, root, 0, args...)
		assertRootFilesEqual(t, directory, beforeReadOnly)
	}

	runResolutionCommand(t, root, 0, "lock")
	refreshedBytes := readRootFile(t, directory, "otelplan.lock")

	refreshed, err := lockfile.Parse(refreshedBytes)
	if err != nil {
		t.Fatal(err)
	}

	if refreshed.Backend.Digest != locked.Backend.Digest || len(refreshed.Artifacts) != 1 ||
		refreshed.Artifacts[0] != locked.Artifacts[0] {
		t.Fatal("lock refresh erased build identity")
	}

	changed := strings.Replace(files["app.go"], "ctx context.Context", "ctx context.Context, value int", 1)

	err = directory.WriteFile("app.go", []byte(changed), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	beforeRejected := snapshotRootFiles(t, directory, files, "otelplan.lock")
	for _, args := range [][]string{
		{"lock", "--check"}, {"diff", "--check"}, {"validate"}, {"lock"}, {"lock", "--dry-run"},
	} {
		runResolutionCommand(t, root, 6, args...)
		assertRootFilesEqual(t, directory, beforeRejected)
	}
}

func runResolutionCommand(t *testing.T, root string, expectedExit int, args ...string) {
	t.Helper()

	arguments := append([]string{"--root", root, "--offline", "--format=json"}, args...)

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), arguments, &stdout, &stderr)
	if exit != expectedExit || stderr.Len() != 0 {
		t.Fatalf("execution args=%v exit=%d, want %d; stdout=%s stderr=%s", args, exit, expectedExit, &stdout, &stderr)
	}

	var reply globalArgumentReply

	err := json.Unmarshal(stdout.Bytes(), &reply)
	if err != nil {
		t.Fatalf("invalid JSON response: %v, %s", err, &stdout)
	}

	if reply.APIVersion != cli.APIVersion || reply.Command != args[0] || reply.OK != (expectedExit == 0) {
		t.Fatalf("invalid response envelope: %+v", reply)
	}

	checkResolutionDiagnostics(t, reply, expectedExit)
}

func checkResolutionDiagnostics(t *testing.T, reply globalArgumentReply, expectedExit int) {
	t.Helper()

	if expectedExit == 0 && len(reply.Diagnostics) != 0 {
		t.Fatalf("successful response has diagnostics: %+v", reply.Diagnostics)
	}

	if expectedExit == 6 && (len(reply.Diagnostics) != 1 || reply.Diagnostics[0].Code != model.CodeStaleLockfile) {
		t.Fatalf("stale resolution response lost its diagnostic: %+v", reply.Diagnostics)
	}
}

func readRootFile(t *testing.T, directory *os.Root, name string) []byte {
	t.Helper()

	contents, err := directory.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	return contents
}

func assertRootFileEqual(t *testing.T, directory *os.Root, name string, want []byte) {
	t.Helper()

	got := readRootFile(t, directory, name)
	if !bytes.Equal(got, want) {
		t.Fatalf("%s changed", name)
	}
}

func snapshotRootFiles(t *testing.T, directory *os.Root, files map[string]string, extra ...string) map[string][]byte {
	t.Helper()

	snapshot := make(map[string][]byte, len(files)+len(extra))
	for name := range files {
		snapshot[name] = readRootFile(t, directory, name)
	}

	for _, name := range extra {
		snapshot[name] = readRootFile(t, directory, name)
	}

	return snapshot
}

func assertRootFilesEqual(t *testing.T, directory *os.Root, want map[string][]byte) {
	t.Helper()

	entries, err := fs.ReadDir(directory.FS(), ".")
	if err != nil {
		t.Fatalf("read project files: %v", err)
	}

	if len(entries) != len(want) {
		t.Fatalf("project file set changed: got %d entries, want %d", len(entries), len(want))
	}

	for _, entry := range entries {
		if _, ok := want[entry.Name()]; !ok {
			t.Fatalf("project file set changed: unexpected %s", entry.Name())
		}
	}

	for name, contents := range want {
		assertRootFileEqual(t, directory, name, contents)
	}
}
