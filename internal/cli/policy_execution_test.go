package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const executionPolicy = `apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
rules:
- id: operation
  match: {functions: [Run]}
`

func TestPolicyExecutionDiagnosticOrder(t *testing.T) {
	t.Parallel()

	policy := executionPolicy + `  attributes:
  - key: password
    from: {constant: caller-private-value}
`
	source := `package main
import "context"
func Run(context.Context) error { return nil }
func main() {}
`
	tests := []struct {
		args  []string
		exit  int
		codes []model.Code
		data  bool
	}{
		{args: []string{"inspect"}, exit: 7,
			codes: []model.Code{model.CodeSecretAttribute, model.CodeBackendUnsupported}, data: true},
		{args: []string{"explain", "example.com/app.Run"}, exit: 7,
			codes: []model.Code{model.CodeSecretAttribute, model.CodeBackendUnsupported}, data: true},
		{args: []string{"explain", "example.com/app.Missing"}, exit: 5,
			codes: []model.Code{model.CodeSecretAttribute, model.CodeBackendUnsupported, model.CodeUnresolvedSymbol},
			data:  false},
	}

	for _, testCase := range tests {
		t.Run(strings.Join(testCase.args, "/"), func(t *testing.T) {
			t.Parallel()

			root, directory, files := policyExecutionFixture(t, source, policy)
			reply := policyExecutionReply(t.Context(), t, root, testCase.args, testCase.exit)
			checkPolicyDiagnosticCodes(t, reply, testCase.codes)
			checkPolicyExecutionData(t, reply, testCase.data)
			checkPolicyExecutionFiles(t, directory, files)
		})
	}
}

func TestPolicyExecutionStrictWarningsPreventWrites(t *testing.T) {
	t.Parallel()

	policy := strings.Replace(executionPolicy, "rules:", "defaults: {context: {mode: root}}\nrules:", 1)
	source := "package app\nfunc Run() error { return nil }\n"

	for _, command := range []string{"inspect", "explain", "validate", "lock", "diff", "compile", "build"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			root, directory, files := policyExecutionFixture(t, source, policy)
			args := []string{command, "--strict"}

			if command == "explain" {
				args = append(args, "example.com/app.Run")
			}

			reply := policyExecutionReply(t.Context(), t, root, args, 5)
			checkPolicyDiagnosticCodes(t, reply, []model.Code{model.CodeMissingContext})
			checkPolicyExecutionData(t, reply, command == "inspect" || command == "explain")

			if reply.Diagnostics[0].Severity != model.SeverityWarning {
				t.Fatalf("strict mode changed warning severity: %+v", reply.Diagnostics)
			}

			checkPolicyExecutionFiles(t, directory, files)
		})
	}
}

func TestPolicyExecutionStagePrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		args   []string
		policy string
		exit   int
		code   model.Code
	}{
		{name: "build arguments before policy loading", args: []string{"build", "--", "-overlay=caller-private-value"},
			policy: "malformed: [", exit: 2, code: model.CodeInvalidPolicy},
		{name: "policy schema before discovery", args: []string{"inspect"},
			policy: strings.Replace(executionPolicy, "otelplan.io/v1alpha1", "unsupported", 1),
			exit:   3, code: model.CodeInvalidPolicy},
		{name: "discovery before resolution", args: []string{"inspect"},
			policy: executionPolicy, exit: 4, code: model.CodeUnresolvedSymbol},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root, directory, files := policyExecutionFixture(t, "this is not valid Go source", testCase.policy)
			reply := policyExecutionReply(t.Context(), t, root, testCase.args, testCase.exit)
			checkPolicyDiagnosticCodes(t, reply, []model.Code{testCase.code})
			checkPolicyExecutionData(t, reply, false)
			checkPolicyExecutionFiles(t, directory, files)
		})
	}
}

func TestPolicyExecutionPropagatesCanceledContext(t *testing.T) {
	t.Parallel()

	root, directory, files := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(context.Context) error { return nil }\n", executionPolicy)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	reply := policyExecutionReply(ctx, t, root, []string{"inspect", "--verbose"}, 4)
	checkPolicyDiagnosticCodes(t, reply, []model.Code{model.CodeUnresolvedSymbol})
	checkPolicyExecutionData(t, reply, false)
	checkPolicyExecutionFiles(t, directory, files)
}

func checkPolicyExecutionData(t *testing.T, reply globalArgumentReply, want bool) {
	t.Helper()

	if (len(reply.Data) > 0) != want {
		t.Fatalf("response data=%s; want data presence %t", reply.Data, want)
	}
}

func policyExecutionFixture(t *testing.T, source, policy string) (string, *os.Root, map[string]string) {
	t.Helper()

	root, directory, files := callGraphFixture(t)
	files["app.go"] = source
	files["otelplan.yaml"] = policy

	for name, contents := range files {
		err := directory.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatalf("write policy execution fixture: %v", err)
		}
	}

	return root, directory, files
}

func policyExecutionReply(ctx context.Context, t *testing.T, root string,
	args []string, expectedExit int,
) globalArgumentReply {
	t.Helper()

	arguments := append([]string{"--root", root, "--offline", "--format=json"}, args...)

	var stdout, stderr bytes.Buffer

	exit := cli.Run(ctx, arguments, &stdout, &stderr)
	if exit != expectedExit || stderr.Len() != 0 || bytes.Contains(stdout.Bytes(), []byte("caller-private-value")) {
		t.Fatalf("execution exit=%d, want %d; stdout=%s stderr=%s", exit, expectedExit, &stdout, &stderr)
	}

	var reply globalArgumentReply

	err := json.Unmarshal(stdout.Bytes(), &reply)
	if err != nil || reply.OK || reply.Command != args[0] || reply.APIVersion != cli.APIVersion {
		t.Fatalf("invalid failed-command envelope: %v, %s", err, &stdout)
	}

	return reply
}

func checkPolicyDiagnosticCodes(t *testing.T, reply globalArgumentReply, codes []model.Code) {
	t.Helper()

	if len(reply.Diagnostics) != len(codes) {
		t.Fatalf("diagnostics=%+v; want codes %v", reply.Diagnostics, codes)
	}

	for index, code := range codes {
		if reply.Diagnostics[index].Code != code {
			t.Fatalf("diagnostics=%+v; want codes %v", reply.Diagnostics, codes)
		}
	}
}

func checkPolicyExecutionFiles(t *testing.T, directory *os.Root, files map[string]string) {
	t.Helper()

	entries, err := os.ReadDir(directory.Name())
	if err != nil || len(entries) != len(files) {
		t.Fatalf("command changed project files: %v, %v", entries, err)
	}

	for name, want := range files {
		got, err := directory.ReadFile(name)
		if err != nil || string(got) != want {
			t.Fatalf("command changed %s: %v", name, err)
		}
	}
}
