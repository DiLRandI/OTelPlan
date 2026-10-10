package cli_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

type globalArgumentReply struct {
	APIVersion  string                    `json:"apiVersion"`
	Command     string                    `json:"command"`
	OK          bool                      `json:"ok"`
	Diagnostics model.DiagnosticErrorList `json:"diagnostics"`
	Data        json.RawMessage           `json:"data"`
}

func TestGlobalArgumentSuccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		command string
	}{
		{name: "before command", args: []string{"--format=json", "version"}, command: "version"},
		{name: "after command", args: []string{"version", "--format", "json"}, command: "version"},
		{name: "single dash", args: []string{"-format=json", "version"}, command: "version"},
		{name: "interspersed values", args: []string{"--root", "path with spaces", "version", "--format=json"},
			command: "version"},
		{name: "dash prefixed value", args: []string{"--root", "-directory", "version", "--format=json"},
			command: "version"},
		{name: "last format wins", args: []string{"--format=text", "version", "--format=json"}, command: "version"},
		{name: "false boolean", args: []string{"version", "--strict=false", "--format=json"}, command: "version"},
		{name: "help alias", args: []string{"version", "-h", "--format=json"}, command: "help"},
		{name: "empty positional tail", args: []string{"version", "--format=json", "--"}, command: "version"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			reply := runGlobalArgumentsJSON(t, testCase.args, 0)
			if !reply.OK || reply.Command != testCase.command || len(reply.Diagnostics) != 0 || len(reply.Data) == 0 {
				t.Fatalf("invalid successful response: %+v", reply)
			}
		})
	}
}

func TestGlobalArgumentErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{name: "format after unknown flag", args: []string{"--unknown=private-value", "version", "--format=json"},
			message: "unknown flag: --unknown"},
		{name: "first error wins", args: []string{"--unknown=private-value", "version", "--format=json", "--config"},
			message: "unknown flag: --unknown"},
		{name: "missing value", args: []string{"version", "--format=json", "--root"},
			message: "flag --root requires a value"},
		{name: "invalid boolean", args: []string{"version", "--strict=private-value", "--format", "json"},
			message: "invalid flag value or syntax"},
		{name: "malformed dash syntax", args: []string{"version", "---format=json"},
			message: "invalid flag value or syntax"},
		{name: "standalone dash", args: []string{"version", "-", "--format=json"},
			message: "version takes no positional arguments"},
		{name: "positional tail", args: []string{"version", "--format=json", "--", "--unknown=private-value"},
			message: "version takes no positional arguments"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			reply := runGlobalArgumentsJSON(t, testCase.args, 2)
			if reply.OK || reply.Command != "version" || len(reply.Diagnostics) != 1 {
				t.Fatalf("invalid usage response: %+v", reply)
			}

			diagnostic := reply.Diagnostics[0]
			if diagnostic.Code != model.CodeInvalidPolicy || diagnostic.Message != testCase.message {
				t.Fatalf("diagnostic = %+v; want message %q", diagnostic, testCase.message)
			}
		})
	}
}

func TestGlobalArgumentTextBoundary(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), []string{"version", "--", "--format=json"}, &stdout, &stderr)
	if exit != 2 || strings.TrimSpace(stdout.String()) != "error OTP1005: version takes no positional arguments" ||
		stderr.Len() != 0 {
		t.Fatalf("double dash did not preserve text mode: exit=%d stdout=%s stderr=%s", exit, &stdout, &stderr)
	}
}

func runGlobalArgumentsJSON(t *testing.T, args []string, expectedExit int) globalArgumentReply {
	t.Helper()

	before := slices.Clone(args)

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), args, &stdout, &stderr)
	if exit != expectedExit || stderr.Len() != 0 {
		t.Fatalf("CLI exit=%d, want %d; stdout=%s stderr=%s", exit, expectedExit, &stdout, &stderr)
	}

	if !slices.Equal(args, before) || strings.Contains(stdout.String(), "private-value") {
		t.Fatal("CLI changed caller arguments or exposed a flag value")
	}

	var reply globalArgumentReply

	err := json.Unmarshal(stdout.Bytes(), &reply)
	if err != nil || reply.APIVersion != cli.APIVersion {
		t.Fatalf("invalid JSON envelope: %v, %s", err, &stdout)
	}

	return reply
}
