package cli_test

import (
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
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

	const linkedVersion = "v0.0.0-contract"

	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary,
		"-ldflags", "-X github.com/DiLRandI/OTelPlan/internal/cli.Version="+linkedVersion, "../../cmd/otelplan")

	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build CLI executable: %v: %s", err, output)
	}

	for _, commandName := range []string{"version", "inspect"} {
		command := exec.CommandContext(t.Context(), binary, commandName, "--root", root, "--format=json")

		output, err = command.CombinedOutput()
		if err != nil {
			t.Fatalf("run %s through CLI executable: %v: %s", commandName, err, output)
		}

		if commandName == "version" {
			assertLinkedVersion(t, output, linkedVersion)
		} else {
			assertInspectionSuccess(t, output, "inspect")
		}

		assertRootFilesEqual(t, directory, before)
	}

	command := exec.CommandContext(t.Context(), binary, "inspect", "--root", root, "--config=missing.yaml")

	output, err = command.CombinedOutput()

	var exit *exec.ExitError

	if !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Fatalf("invalid policy process exit: %v: %s", err, output)
	}

	assertRootFilesEqual(t, directory, before)
}

func assertLinkedVersion(t *testing.T, output []byte, want string) {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil {
		t.Fatalf("decode linked-version response: %v", err)
	}

	if reply.APIVersion != cli.APIVersion || reply.Command != "version" || !reply.OK || len(reply.Diagnostics) != 0 {
		t.Fatalf("invalid linked-version response: %+v", reply)
	}

	var versions map[string]string

	err = json.Unmarshal(reply.Data, &versions)
	if err != nil {
		t.Fatalf("decode linked-version data: %v", err)
	}

	if versions["otelplan"] != want || versions["go"] != runtime.Version() {
		t.Fatalf("linked tool identity changed: got %v, want otelplan=%s and go=%s", versions, want, runtime.Version())
	}
}
