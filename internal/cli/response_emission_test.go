package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
)

func TestResponseTextWorkflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "scan", args: []string{"scan"}, want: "example.com/app.Ignored\n" +
			"  signature func(ctx context.Context) error\n  source app.go:4\n  context [0]  errors [0]\n" +
			"example.com/app.Run\n  signature func(ctx context.Context, input string) (output string, err error)\n" +
			"  source app.go:3\n  context [0]  errors [1]\n"},
		{name: "inspect", args: []string{"inspect"}, want: "SELECTED example.com/app.Run\n" +
			"  span app.Run\n  context argument[0]\n  errors record=true indexes=[1]\n  rule operation\n" +
			"  attribute component from constant\n  attribute request.kind from argument input\n" +
			"  attribute response.kind from result output\nSKIPPED example.com/app.Ignored\n  rule operation: excluded\n"},
		{name: "explain selected", args: []string{"explain", "example.com/app.Run"},
			want: "example.com/app.Run selected=true\n  operation include: selector matched\n" +
				"  operation local-exclusion: selector did not match\n"},
		{name: "explain excluded", args: []string{"explain", "example.com/app.Ignored"},
			want: "example.com/app.Ignored selected=false\n  operation include: selector matched\n" +
				"  operation local-exclusion: selector matched\n"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := responseTextFixture(t)
			arguments := append([]string{"--root", root, "--offline"}, testCase.args...)

			var stdout, stderr bytes.Buffer

			exit := cli.Run(t.Context(), arguments, &stdout, &stderr)
			if exit != 0 || stdout.String() != testCase.want || stderr.Len() != 0 {
				t.Fatalf("exit=%d stdout=%q stderr=%q; want %q", exit, &stdout, &stderr, testCase.want)
			}
		})
	}
}

func TestResponseQuietText(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), []string{"version", "--quiet"}, &stdout, &stderr)
	if exit != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("quiet text version exit=%d stdout=%s stderr=%s", exit, &stdout, &stderr)
	}
}

func TestResponseQuietDiagnostics(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), []string{"version", "--quiet", "--check"}, &stdout, &stderr)
	if exit != 2 || stdout.String() != "error OTP1005: --check is supported by lock and diff\n" || stderr.Len() != 0 {
		t.Fatalf("quiet suppressed diagnostics: exit=%d stdout=%s stderr=%s", exit, &stdout, &stderr)
	}
}

func TestResponseQuietJSON(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), []string{"version", "--quiet", "--format=json"}, &stdout, &stderr)

	var reply globalArgumentReply

	err := json.Unmarshal(stdout.Bytes(), &reply)
	if exit != 0 || err != nil || !reply.OK || reply.Command != "version" || len(reply.Data) == 0 || stderr.Len() != 0 {
		t.Fatalf("quiet JSON lost the response: exit=%d stdout=%s stderr=%s error=%v", exit, &stdout, &stderr, err)
	}

	if !bytes.HasPrefix(stdout.Bytes(), []byte("{\n  \"apiVersion\":")) ||
		!bytes.HasSuffix(stdout.Bytes(), []byte("}\n")) {
		t.Fatalf("JSON indentation or trailing newline changed: %q", &stdout)
	}
}

func TestResponseWriteFailureContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		context string
	}{
		{name: "text data", args: []string{"version"}, context: "write text response"},
		{name: "JSON data", args: []string{"version", "--format=json"}, context: "encode JSON response"},
		{name: "diagnostic", args: []string{"version", "--check"}, context: "write diagnostic"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			failure := io.ErrClosedPipe
			writer := responseFailureWriter{err: failure}

			var stderr bytes.Buffer

			exit := cli.Run(t.Context(), testCase.args, writer, &stderr)
			want := fmt.Sprintf("%s: %s\n", testCase.context, failure)

			if exit != 1 || stderr.String() != want {
				t.Fatalf("write failure exit=%d stderr=%q; want %q", exit, &stderr, want)
			}
		})
	}
}

type responseFailureWriter struct {
	err error
}

func (writer responseFailureWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

func responseTextFixture(t *testing.T) string {
	t.Helper()

	root, directory, _ := callGraphFixture(t)
	files := map[string]string{
		"app.go": `package app
import "context"
func Run(ctx context.Context, input string) (output string, err error) { return input, nil }
func Ignored(ctx context.Context) error { return nil }
`,
		"otelplan.yaml": `apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
rules:
- id: operation
  match: {functions: [Run, Ignored]}
  exclude: {functions: [Ignored]}
  attributes:
  - key: component
    from: {constant: caller-private-value}
  - key: request.kind
    from: {argument: input}
  - key: response.kind
    from: {result: output}
`,
	}

	for name, contents := range files {
		err := directory.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatalf("write response fixture: %v", err)
		}
	}

	return root
}
