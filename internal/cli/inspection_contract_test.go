package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestInspectionCommandsJSONAndImmutability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "scan", args: []string{"scan", "./..."}},
		{name: "inspect", args: []string{"inspect"}},
		{name: "explain", args: []string{"explain", "example.com/app.Run"}},
		{name: "version", args: []string{"version"}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root, directory, files := policyExecutionFixture(t,
				"package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n",
				executionPolicy)
			before := snapshotRootFiles(t, directory, files)

			args := append([]string{"--root", root, "--offline", "--format=json"}, testCase.args...)

			var stdout, stderr bytes.Buffer

			exit := cli.Run(t.Context(), args, &stdout, &stderr)
			if exit != 0 || stderr.Len() != 0 {
				t.Fatalf("%v exit=%d stderr=%s stdout=%s", args, exit, &stderr, &stdout)
			}

			assertInspectionSuccess(t, stdout.Bytes(), testCase.args[0])
			assertRootFilesEqual(t, directory, before)
		})
	}
}

func TestCLIExitCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		exit int
	}{
		{name: "unknown flag", args: []string{"--unknown"}, exit: 2},
		{name: "invalid format", args: []string{"--format=xml", "scan"}, exit: 2},
		{name: "missing explain symbol", args: []string{"explain"}, exit: 2},
		{name: "missing config", args: []string{"--config=missing.yaml", "inspect"}, exit: 3},
		{name: "missing scan package", args: []string{"scan", "./missing"}, exit: 4},
		{name: "missing explain target", args: []string{"explain", "example.com/app.Missing"}, exit: 5},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root, directory, files := policyExecutionFixture(t,
				"package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n",
				executionPolicy)
			before := snapshotRootFiles(t, directory, files)

			args := append([]string{"--root", root, "--offline"}, testCase.args...)

			var stdout, stderr bytes.Buffer

			exit := cli.Run(t.Context(), args, &stdout, &stderr)
			if exit != testCase.exit {
				t.Fatalf("%v exit=%d want=%d output=%s stderr=%s", testCase.args, exit, testCase.exit, &stdout, &stderr)
			}

			assertRootFilesEqual(t, directory, before)
		})
	}
}

func TestCLIOutputFailure(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	args := []string{"version", "--format=json"}

	exit := cli.Run(t.Context(), args, responseFailureWriter{err: io.ErrClosedPipe}, &stderr)
	if exit != 1 {
		t.Fatalf("output failure exit=%d", exit)
	}
}

func TestInspectRejectsUnsafeCaptureWithoutPrintingConstant(t *testing.T) {
	t.Parallel()

	root, directory, files := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n",
		executionPolicy)
	files["otelplan.yaml"] += "  attributes:\n  - key: password\n    from:\n      constant: do-not-print-this-secret\n"

	err := directory.WriteFile("otelplan.yaml", []byte(files["otelplan.yaml"]), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	before := snapshotRootFiles(t, directory, files)

	var stdout, stderr bytes.Buffer

	args := []string{"inspect", "--root", root, "--offline", "--format=json"}

	exit := cli.Run(t.Context(), args, &stdout, &stderr)
	if exit != 5 {
		t.Fatalf("unsafe inspection exit=%d output=%s stderr=%s", exit, &stdout, &stderr)
	}

	if strings.Contains(stdout.String(), "do-not-print-this-secret") ||
		strings.Contains(stderr.String(), "do-not-print-this-secret") {
		t.Fatal("unsafe constant printed")
	}

	assertUnsafeCaptureDiagnostic(t, stdout.Bytes())
	assertRootFilesEqual(t, directory, before)
}

func assertInspectionSuccess(t *testing.T, output []byte, command string) {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil {
		t.Fatalf("invalid inspection response: %v, %s", err, output)
	}

	if reply.APIVersion != cli.APIVersion || reply.Command != command || !reply.OK || len(reply.Diagnostics) != 0 ||
		len(reply.Data) == 0 || string(reply.Data) == "null" {
		t.Fatalf("invalid inspection response: %+v", reply)
	}
}

func assertUnsafeCaptureDiagnostic(t *testing.T, output []byte) {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil {
		t.Fatalf("invalid unsafe response: %v, %s", err, output)
	}

	if reply.APIVersion != cli.APIVersion || reply.Command != "inspect" || reply.OK || len(reply.Diagnostics) != 1 {
		t.Fatalf("invalid unsafe response envelope: %+v", reply)
	}

	diagnostic := reply.Diagnostics[0]
	if diagnostic.Severity != model.SeverityError || diagnostic.Code != model.CodeSecretAttribute ||
		string(diagnostic.Symbol) != "example.com/app.Run" || diagnostic.RuleID != "operation" {
		t.Fatalf("invalid unsafe diagnostic: %+v", diagnostic)
	}
}
