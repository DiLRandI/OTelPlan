package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/model"
)

const fullyExcludedPolicy = executionPolicy + `exclusions:
- id: excluded
  match: {functions: [Run]}
`

func TestFullyExcludedRulesRespectStrictMode(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"inspect", "explain", "validate", "lock", "diff", "compile", "build"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			root, directory, files := policyExecutionFixture(t,
				"package app\nimport \"context\"\nfunc Run(context.Context) error { return nil }\n", fullyExcludedPolicy)

			args := []string{command, "--strict"}
			switch command {
			case "explain":
				args = append(args, "example.com/app.Run")
			case "build":
				args = append(args, "--locked")
			}

			reply := policyExecutionReply(t.Context(), t, root, args, 5)
			checkPolicyDiagnosticCodes(t, reply, []model.Code{"OTP1006"})

			if reply.Diagnostics[0].Severity != model.SeverityWarning || reply.Diagnostics[0].RuleID != "operation" {
				t.Fatalf("strict mode changed the excluded-rule warning: %+v", reply.Diagnostics)
			}

			checkPolicyExecutionFiles(t, directory, files)
		})
	}
}

func TestFullyExcludedRulesWarnWithoutStrictMode(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()

			root, directory, files := policyExecutionFixture(t,
				"package app\nimport \"context\"\nfunc Run(context.Context) error { return nil }\n", fullyExcludedPolicy)

			var stdout, stderr bytes.Buffer

			exit := cli.Run(t.Context(), []string{"inspect", "--root", root, "--offline", "--format=" + format},
				&stdout, &stderr)
			if exit != 0 || stderr.Len() != 0 {
				t.Fatalf("non-strict excluded rule failed: exit=%d stdout=%s stderr=%s", exit, &stdout, &stderr)
			}

			checkExcludedRuleOutput(t, stdout.Bytes(), format)
			checkPolicyExecutionFiles(t, directory, files)
		})
	}
}

func checkExcludedRuleOutput(t *testing.T, output []byte, format string) {
	t.Helper()

	if format == "text" {
		want := "warning OTP1006: all matched symbols are excluded; rule selects no targets (rule operation)"
		if !strings.Contains(string(output), want) {
			t.Fatalf("missing excluded-rule text warning: %s", output)
		}

		return
	}

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil || !reply.OK || reply.Command != "inspect" {
		t.Fatalf("invalid successful warning envelope: %v, %s", err, output)
	}

	checkPolicyDiagnosticCodes(t, reply, []model.Code{"OTP1006"})
}
