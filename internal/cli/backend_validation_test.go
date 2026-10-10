package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestUnsupportedBackendTargetsFailValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		source  string
		message string
		symbol  string
	}{
		{
			name:    "main",
			source:  "package main\nimport \"context\"\nfunc Run(context.Context) error { return nil }\nfunc main(){}\n",
			message: "main package targets require verified command-specific build scoping",
			symbol:  "example.com/app.Run",
		},
		{
			name: "variadic named type",
			source: "package app\nimport \"context\"\ntype Value string\n" +
				"func Run(context.Context, ...Value) error { return nil }\n",
			message: "variadic targets require a built-in element type that generated hooks can name safely",
			symbol:  "example.com/app.Run",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for _, command := range []string{"validate", "inspect", "compile", "build"} {
				for _, format := range []string{"text", "json"} {
					t.Run(command+"/"+format, func(t *testing.T) {
						t.Parallel()

						root, directory, files := policyExecutionFixture(t, testCase.source, executionPolicy)
						before := snapshotRootFiles(t, directory, files)

						var stdout, stderr bytes.Buffer

						args := []string{"--root", root, "--offline", "--format=" + format, command}

						exit := cli.Run(t.Context(), args, &stdout, &stderr)
						if exit != 7 || stderr.Len() != 0 {
							t.Fatalf("%s %s exit=%d, want 7: stdout=%s stderr=%s", command, format, exit, &stdout, &stderr)
						}

						assertBackendUnsupportedText(t, stdout.String(), testCase.message)

						if format == "json" {
							assertBackendUnsupportedJSON(t, stdout.Bytes(), command, command == "inspect", testCase.symbol, testCase.message)
						}

						assertRootFilesEqual(t, directory, before)
					})
				}
			}
		})
	}
}

func assertBackendUnsupportedText(t *testing.T, output, message string) {
	t.Helper()

	if !strings.Contains(output, string(model.CodeBackendUnsupported)) || !strings.Contains(output, message) {
		t.Fatalf("missing compatibility diagnostic: %s", output)
	}
}

func assertBackendUnsupportedJSON(t *testing.T, output []byte, command string, wantData bool, symbol, message string) {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil {
		t.Fatalf("invalid response: %v, %s", err, output)
	}

	assertBackendUnsupportedEnvelope(t, reply, command)
	assertBackendUnsupportedDiagnostic(t, reply.Diagnostics[0], symbol, message)

	if (len(reply.Data) > 0) != wantData {
		t.Fatalf("response data=%s; want data presence %t", reply.Data, wantData)
	}
}

func assertBackendUnsupportedEnvelope(t *testing.T, reply globalArgumentReply, command string) {
	t.Helper()

	if reply.APIVersion != cli.APIVersion || reply.Command != command || reply.OK || len(reply.Diagnostics) != 1 {
		t.Fatalf("invalid response envelope: %+v", reply)
	}
}

func assertBackendUnsupportedDiagnostic(t *testing.T, diagnostic model.DiagnosticError, symbol, message string) {
	t.Helper()

	if diagnostic.Severity != model.SeverityError || diagnostic.Code != model.CodeBackendUnsupported ||
		string(diagnostic.Symbol) != symbol || diagnostic.RuleID != "operation" || diagnostic.Message != message {
		t.Fatalf("unexpected compatibility diagnostic: %+v", diagnostic)
	}
}
