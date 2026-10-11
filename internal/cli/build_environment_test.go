package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestLockChecksPreserveExplicitModModeInputs(t *testing.T) {
	root, directory, files := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n", executionPolicy)
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOWORK", "off")

	files["go.mod"] += "\nreplace example.com/local => ./local\n"
	files["dependency.go"] = "package app\nimport _ \"example.com/local\"\n"
	files["local/go.mod"] = "module example.com/local\n\ngo 1.27\n"
	files["local/local.go"] = "package local\n"

	for name, contents := range files {
		err := directory.MkdirAll(filepath.Dir(name), 0o700)
		if err != nil {
			t.Fatalf("create build-environment fixture directory: %v", err)
		}

		err = directory.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatalf("write build-environment fixture %s: %v", name, err)
		}
	}

	before := snapshotBuildEnvironmentFiles(t, directory)
	runResolutionCommand(t, root, 0, "lock")
	before["otelplan.lock"] = readRootFile(t, directory, "otelplan.lock")
	assertBuildEnvironmentFilesUnchanged(t, directory, before)

	for _, args := range [][]string{{"lock", "--check"}, {"diff", "--check"}} {
		runResolutionCommand(t, root, 0, args...)
		assertBuildEnvironmentFilesUnchanged(t, directory, before)
	}

	_, err := directory.Stat("go.sum")
	if !os.IsNotExist(err) {
		t.Fatalf("analysis created a project go.sum or made it unreadable: %v", err)
	}
}

func TestLockChecksDetectAmbientBuildTags(t *testing.T) {
	root, directory, _ := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n", executionPolicy)
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-tags=first")

	before := snapshotBuildEnvironmentFiles(t, directory)
	runResolutionCommand(t, root, 0, "lock")
	before["otelplan.lock"] = readRootFile(t, directory, "otelplan.lock")
	assertBuildEnvironmentFilesUnchanged(t, directory, before)

	for _, args := range [][]string{{"lock", "--check"}, {"diff", "--check"}} {
		runResolutionCommand(t, root, 0, args...)
		assertBuildEnvironmentFilesUnchanged(t, directory, before)
	}

	t.Setenv("GOFLAGS", "-tags=second")

	for _, command := range []string{"lock", "diff"} {
		assertBuildTagDrift(t, root, command)
		assertBuildEnvironmentFilesUnchanged(t, directory, before)
	}
}

func assertBuildTagDrift(t *testing.T, root, command string) {
	t.Helper()

	args := []string{command, "--check", "--root", root, "--offline", "--format=json"}
	reply := runGlobalArgumentsJSON(t, args, 6)

	if reply.Command != command || reply.OK {
		t.Fatalf("invalid build-tag drift response envelope: %+v", reply)
	}

	checkResolutionDiagnostics(t, reply, 6)

	var diff model.LockDiff

	err := json.Unmarshal(reply.Data, &diff)
	if err != nil {
		t.Fatalf("decode build-tag drift diff: %v", err)
	}

	if len(diff.Entries) != 1 || diff.Entries[0].Classification != model.DiffBuild ||
		diff.Entries[0].Symbol != "" || diff.Entries[0].Detail == "" {
		t.Fatalf("unexpected build-tag drift: got %+v, want one global BUILD entry", diff)
	}
}

func snapshotBuildEnvironmentFiles(t *testing.T, directory *os.Root) map[string][]byte {
	t.Helper()

	files := map[string][]byte{}

	err := fs.WalkDir(directory.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk build-environment fixture: %w", walkErr)
		}

		if entry.IsDir() {
			return nil
		}

		contents, readErr := directory.ReadFile(name)
		if readErr != nil {
			return fmt.Errorf("read build-environment fixture %s: %w", name, readErr)
		}

		files[name] = contents

		return nil
	})
	if err != nil {
		t.Fatalf("snapshot build-environment fixture: %v", err)
	}

	return files
}

func assertBuildEnvironmentFilesUnchanged(t *testing.T, directory *os.Root, want map[string][]byte) {
	t.Helper()

	got := snapshotBuildEnvironmentFiles(t, directory)
	if len(got) != len(want) {
		t.Fatalf("build-environment analysis changed the project file count: got %d, want %d", len(got), len(want))
	}

	for name, contents := range want {
		actual, exists := got[name]
		if !exists || !bytes.Equal(actual, contents) {
			t.Fatalf("build-environment analysis changed or removed project file %s", name)
		}
	}
}
