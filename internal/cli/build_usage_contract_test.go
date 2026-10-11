package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestBuildCLIUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{name: "missing output", args: []string{"build", "--", "-o"}, message: "build flag -o requires a value"},
		{name: "source override", args: []string{"build", "--", "-overlay=secret"},
			message: "unsupported build flag; source-selection and compiler overrides require analysis support"},
		{name: "check applicability", args: []string{"build", "--check"},
			message: "--check is supported by lock and diff"},
		{name: "clean applicability", args: []string{"build", "--clean"},
			message: "--clean is supported by compile"},
	}
	for _, testCase := range tests {
		for _, format := range []string{"text", "json"} {
			t.Run(testCase.name+"/"+format, func(t *testing.T) {
				t.Parallel()

				root, directory, files := policyExecutionFixture(t,
					"package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n", executionPolicy)
				before := snapshotRootFiles(t, directory, files)

				args := append([]string{"--root", root, "--offline", "--format=" + format}, testCase.args...)

				var stdout, stderr bytes.Buffer

				exit := cli.Run(t.Context(), args, &stdout, &stderr)
				if exit != 2 || stderr.Len() != 0 || strings.Contains(stdout.String(), "secret") {
					t.Fatalf("invalid build-usage response: exit=%d stdout=%s stderr=%s", exit, &stdout, &stderr)
				}

				if format == "json" {
					assertBuildUsageJSON(t, stdout.Bytes(), testCase.message)
				} else {
					want := "error " + string(model.CodeInvalidPolicy) + ": " + testCase.message
					if strings.TrimSpace(stdout.String()) != want {
						t.Fatalf("wrong build-usage diagnostic: got %q, want %q", stdout.String(), want)
					}
				}

				assertRootFilesEqual(t, directory, before)
			})
		}
	}
}

func assertBuildUsageJSON(t *testing.T, output []byte, message string) {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil {
		t.Fatalf("decode build-usage JSON: %v", err)
	}

	if reply.APIVersion != cli.APIVersion || reply.Command != "build" || reply.OK ||
		len(reply.Diagnostics) != 1 || len(reply.Data) != 0 {
		t.Fatalf("invalid build-usage JSON envelope: %+v", reply)
	}

	diagnostic := reply.Diagnostics[0]
	if diagnostic.Code != model.CodeInvalidPolicy || diagnostic.Severity != model.SeverityError ||
		diagnostic.Message != message {
		t.Fatalf("wrong build-usage JSON diagnostic: %+v, want %q", diagnostic, message)
	}
}
