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

func TestCLIUsageContracts(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		args    []string
		message string
	}{
		{"unknown flag", []string{"lock", "--unknown=private-value"}, "unknown flag"},
		{"missing value", []string{"lock", "--config"}, "requires a value"},
		{"invalid boolean", []string{"lock", "--strict=invalid"}, "invalid"},
		{"missing command", nil, "usage:"},
		{"lock positional", []string{"lock", "foo"}, "lock takes no positional arguments"},
		{"validate positional", []string{"validate", "foo"}, "validate takes no positional arguments"},
		{"conflicting flags", []string{"lock", "--check", "--dry-run"}, "cannot combine"},
		{"dependencies applicability", []string{"inspect", "--dependencies"}, "scan"},
		{"interfaces applicability", []string{"lock", "--interfaces"}, "scan"},
		{"strict applicability", []string{"scan", "--strict"}, "policy"},
		{"config applicability", []string{"scan", "--config=unused.yaml"}, "policy"},
	} {
		for _, format := range []string{"text", "json"} {
			t.Run(testCase.name+"/"+format, func(t *testing.T) {
				t.Parallel()

				args := append([]string{"--root", t.TempDir(), "--format=" + format}, testCase.args...)

				var out, errout bytes.Buffer

				if code := cli.Run(t.Context(), args, &out, &errout); code != 2 {
					t.Fatalf("usage exit=%d output=%s stderr=%s", code, &out, &errout)
				}

				if !strings.Contains(out.String()+errout.String(), testCase.message) {
					t.Fatalf("wrong diagnostic: %s %s", &out, &errout)
				}

				if strings.Contains(out.String()+errout.String(), "private-value") {
					t.Fatal("argument value leaked")
				}

				if format == "json" {
					checkUsageJSONEnvelope(t, &out, &errout)
				}
			})
		}
	}
}

func TestJSONFormatAfterInvalidFlag(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"--unknown", "lock", "--format=json"}, {"lock", "--strict=invalid", "--format", "json"},
	} {
		var out, errout bytes.Buffer
		if code := cli.Run(t.Context(), args, &out, &errout); code != 2 {
			t.Fatalf("exit=%d", code)
		}

		var reply globalArgumentReply

		err := json.Unmarshal(out.Bytes(), &reply)
		if err != nil || reply.OK || len(reply.Diagnostics) == 0 || errout.Len() != 0 {
			t.Fatalf("invalid JSON error: %s %s", &out, &errout)
		}
	}
}

func TestDiffCheckFailureHasDiagnostic(t *testing.T) {
	t.Parallel()

	root, directory, files := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(context.Context) error { return nil }\n", executionPolicy)
	reply := policyExecutionReply(t.Context(), t, root, []string{"diff", "--check"}, 6)

	if len(reply.Diagnostics) == 0 || len(reply.Data) == 0 {
		t.Fatalf("stale diff lacks diagnostic or data: %+v", reply)
	}

	if reply.Diagnostics[0].Code != model.CodeStaleLockfile {
		t.Fatalf("stale diff lost its diagnostic code: %+v", reply.Diagnostics)
	}

	checkPolicyExecutionFiles(t, directory, files)
}

func TestUsageJSONOutputFailure(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	args := []string{"--format=json", "--unknown"}
	if got := cli.Run(t.Context(), args, responseFailureWriter{err: io.ErrClosedPipe}, &stderr); got != 1 {
		t.Fatalf("write failure exit=%d", got)
	}
}

func TestDiffCheckTextDiagnostic(t *testing.T) {
	t.Parallel()

	root, directory, files := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(context.Context) error { return nil }\n", executionPolicy)

	var out, errout bytes.Buffer

	if got := cli.Run(t.Context(), []string{"diff", "--check", "--root", root, "--offline"}, &out, &errout); got != 6 {
		t.Fatalf("exit=%d output=%s stderr=%s", got, &out, &errout)
	}

	if !strings.Contains(out.String(), "OTP6001") || !strings.Contains(out.String(), "lockfile") {
		t.Fatalf("missing stale diagnostic: %s", &out)
	}

	checkPolicyExecutionFiles(t, directory, files)
}

func TestUsageTextOutputFailure(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer

	args := []string{"--unknown"}
	if got := cli.Run(t.Context(), args, &stdout, responseFailureWriter{err: io.ErrClosedPipe}); got != 1 {
		t.Fatalf("diagnostic write failure exit=%d", got)
	}
}

func checkUsageJSONEnvelope(t *testing.T, output, stderr *bytes.Buffer) {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output.Bytes(), &reply)
	if err != nil || reply.OK || len(reply.Diagnostics) == 0 || stderr.Len() != 0 || reply.APIVersion != cli.APIVersion {
		t.Fatalf("invalid error envelope: %v, %s stderr=%s", err, output, stderr)
	}
}
