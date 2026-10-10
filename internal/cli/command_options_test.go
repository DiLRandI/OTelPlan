package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestCommandOptionErrorPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{name: "check and dry run conflict first", args: []string{"scan", "--check", "--dry-run", "--strict"},
			message: "cannot combine --check and --dry-run"},
		{name: "scan flags before policy flags", args: []string{"version", "--dependencies", "--strict"},
			message: "--dependencies, --interfaces, and --calls are supported by scan"},
		{name: "policy flags before output flags", args: []string{"scan", "--config=caller-private-value", "--output=x"},
			message: "--config, --strict, and --allow-large-plan require a policy command"},
		{name: "empty compile output before init flags", args: []string{"compile", "--output=", "--force"},
			message: "--output must not be empty"},
		{name: "output applicability before clean", args: []string{"lock", "--output=x", "--clean"},
			message: "--output is supported by init and compile"},
		{name: "clean before interactive conflicts", args: []string{"init", "--clean", "--interactive", "--non-interactive"},
			message: "--clean is supported by compile"},
		{name: "init ownership before interactive conflicts", args: []string{"scan", "--interactive", "--non-interactive"},
			message: "--force, --interactive, and --non-interactive are supported by init"},
		{name: "interactive conflict before JSON restriction", args: []string{"init", "--interactive", "--non-interactive"},
			message: "cannot combine --interactive and --non-interactive"},
		{name: "check applicability", args: []string{"version", "--check", "--dry-run=false"},
			message: "--check is supported by lock and diff"},
		{name: "dry run applicability", args: []string{"diff", "--dry-run"},
			message: "--dry-run is supported by lock"},
	}

	for _, testCase := range tests {
		for _, format := range []string{"text", "json"} {
			t.Run(testCase.name+"/"+format, func(t *testing.T) {
				t.Parallel()

				checkCommandOptionFailure(t, testCase.args, format, testCase.message)
			})
		}
	}
}

func TestCommandOptionInteractivePrerequisites(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format  string
		message string
	}{
		{format: "text", message: "--interactive requires an input stream"},
		{format: "json", message: "--interactive requires text output"},
	}

	for _, testCase := range tests {
		t.Run(testCase.format, func(t *testing.T) {
			t.Parallel()

			checkCommandOptionFailure(t, []string{"init", "--interactive"}, testCase.format, testCase.message)
		})
	}
}

func checkCommandOptionFailure(t *testing.T, args []string, format, message string) {
	t.Helper()

	root := t.TempDir()
	arguments := append([]string{"--root", root, "--format=" + format}, args...)

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), arguments, &stdout, &stderr)
	if exit != 2 || stderr.Len() != 0 || strings.Contains(stdout.String(), "caller-private-value") {
		t.Fatalf("usage exit=%d stdout=%s stderr=%s", exit, &stdout, &stderr)
	}

	if format == "json" {
		checkCommandOptionJSON(t, stdout.Bytes(), args[0], message)
	} else if stdout.String() != "error OTP1005: "+message+"\n" {
		t.Fatalf("usage text=%q; want message %q", &stdout, message)
	}

	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("usage error created project files: %v, %v", entries, err)
	}
}

func checkCommandOptionJSON(t *testing.T, output []byte, command, message string) {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil || reply.OK || reply.Command != command || len(reply.Diagnostics) != 1 {
		t.Fatalf("invalid usage JSON: %v, %s", err, output)
	}

	diagnostic := reply.Diagnostics[0]
	if diagnostic.Code != model.CodeInvalidPolicy || diagnostic.Message != message {
		t.Fatalf("diagnostic=%+v; want message %q", diagnostic, message)
	}
}
