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

func TestCommandDispatchBuiltins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		command string
		exit    int
		message string
	}{
		{name: "help", args: []string{"help"}, command: "help", exit: 0, message: ""},
		{name: "version", args: []string{"version"}, command: "version", exit: 0, message: ""},
		{name: "help positional", args: []string{"help", "extra"}, command: "help", exit: 2,
			message: "help takes no positional arguments"},
		{name: "version positional", args: []string{"version", "extra"}, command: "version", exit: 2,
			message: "version takes no positional arguments"},
		{name: "unknown command", args: []string{"missing"}, command: "missing", exit: 2,
			message: "unknown command: missing"},
		{name: "help override", args: []string{"missing", "extra", "--help"}, command: "help", exit: 0, message: ""},
		{name: "option validation before unknown command", args: []string{"missing", "--strict"},
			command: "missing", exit: 2, message: "--config, --strict, and --allow-large-plan require a policy command"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			args := append([]string{"--format=json"}, testCase.args...)

			exit := cli.Run(t.Context(), args, &stdout, &stderr)
			if exit != testCase.exit || stderr.Len() != 0 {
				t.Fatalf("dispatch exit=%d stdout=%s stderr=%s", exit, &stdout, &stderr)
			}

			var reply globalArgumentReply

			err := json.Unmarshal(stdout.Bytes(), &reply)
			if err != nil || reply.Command != testCase.command || reply.OK != (exit == 0) {
				t.Fatalf("invalid dispatch response: %v, %s", err, &stdout)
			}

			checkDispatchPayload(t, reply, testCase.message)
		})
	}
}

func checkDispatchPayload(t *testing.T, reply globalArgumentReply, message string) {
	t.Helper()

	if message == "" {
		if len(reply.Diagnostics) != 0 || len(reply.Data) == 0 {
			t.Fatalf("successful dispatch lost data or added diagnostics: %+v", reply)
		}

		return
	}

	if len(reply.Data) != 0 || len(reply.Diagnostics) != 1 {
		t.Fatalf("usage dispatch data or diagnostic count changed: %+v", reply)
	}

	diagnostic := reply.Diagnostics[0]
	if diagnostic.Code != model.CodeInvalidPolicy || diagnostic.Message != message {
		t.Fatalf("usage diagnostic=%+v; want message %q", diagnostic, message)
	}
}

func TestCommandDispatchPreservesStreamOwnership(t *testing.T) {
	t.Parallel()

	input := new(dispatchInput)
	stdout := new(dispatchOutput)
	stderr := new(dispatchOutput)

	exit := cli.RunWithInput(t.Context(), []string{"help"}, input, stdout, stderr)
	if exit != 0 || input.closed || stdout.closed || stderr.closed {
		t.Fatalf("dispatch closed caller streams: exit=%d input=%t stdout=%t stderr=%t",
			exit, input.closed, stdout.closed, stderr.closed)
	}

	if !strings.HasPrefix(string(stdout.data), "usage: otelplan") || len(stderr.data) != 0 {
		t.Fatalf("unexpected help output: stdout=%s stderr=%s", stdout.data, stderr.data)
	}
}

type dispatchInput struct {
	closed bool
}

func (*dispatchInput) Read([]byte) (int, error) { return 0, io.EOF }

func (input *dispatchInput) Close() error {
	input.closed = true

	return nil
}

type dispatchOutput struct {
	data   []byte
	closed bool
}

func (output *dispatchOutput) Write(data []byte) (int, error) {
	output.data = append(output.data, data...)

	return len(data), nil
}

func (output *dispatchOutput) Close() error {
	output.closed = true

	return nil
}
