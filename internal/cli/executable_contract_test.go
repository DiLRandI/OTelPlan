package cli_test

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCLIExecutable(t *testing.T) {
	t.Parallel()

	policy := "apiVersion: otelplan.io/v1alpha1\n" +
		"kind: InstrumentationPlan\n" +
		"backend: {name: otelc, version: v1.1.0}\n" +
		"rules:\n- id: operation\n  match:\n    functions: [Run]\n"
	root, directory, files := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n", policy)
	before := snapshotRootFiles(t, directory, files)

	binary := filepath.Join(t.TempDir(), "otelplan")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "../../cmd/otelplan")

	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build CLI executable: %v: %s", err, output)
	}

	command := exec.CommandContext(t.Context(), binary, "inspect", "--root", root, "--format=json")

	output, err = command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect through CLI executable: %v: %s", err, output)
	}

	assertInspectionSuccess(t, output, "inspect")
	assertRootFilesEqual(t, directory, before)

	command = exec.CommandContext(t.Context(), binary, "inspect", "--root", root, "--config=missing.yaml")

	output, err = command.CombinedOutput()

	var exit *exec.ExitError

	if !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Fatalf("invalid policy process exit: %v: %s", err, output)
	}

	assertRootFilesEqual(t, directory, before)
}
