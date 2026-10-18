package cli_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestTargetLimitFlagsAndStrictWarnings(t *testing.T) {
	t.Parallel()

	root, directory, files := policyExecutionFixture(t, targetLimitSource(), targetLimitPolicy())

	checks := []struct {
		flags    []string
		warnings int
		exit     int
	}{
		{[]string{"--warn-targets=3", "--strong-warn-targets=3"}, 0, 0},
		{[]string{"--warn-targets=2", "--strong-warn-targets=3"}, 1, 5},
		{[]string{"--warn-targets=1", "--strong-warn-targets=2"}, 2, 5},
	}
	for _, check := range checks {
		args := append([]string{"validate", "--strict"}, check.flags...)

		reply := invokeTargetLimit(t, root, args, check.exit)
		if len(reply.Diagnostics) != check.warnings {
			t.Fatalf("args=%v diagnostics=%+v", args, reply.Diagnostics)
		}

		for _, diagnostic := range reply.Diagnostics {
			if diagnostic.Code != model.CodeBroadPlan || diagnostic.Severity != model.SeverityWarning {
				t.Fatalf("invalid threshold warning: %+v", diagnostic)
			}
		}

		arguments := append([]string{"--root", root, "--offline"}, args...)

		var stdout, stderr bytes.Buffer
		if exit := cli.Run(t.Context(), arguments, &stdout, &stderr); exit != check.exit || stderr.Len() != 0 {
			t.Fatalf("text threshold check exit=%d stdout=%s stderr=%s", exit, &stdout, &stderr)
		}

		if strings.Count(stdout.String(), string(model.CodeBroadPlan)) != check.warnings {
			t.Fatalf("text warnings=%s; want %d", &stdout, check.warnings)
		}
	}

	invokeTargetLimit(t, root, []string{"validate", "--warn-targets=1", "--strong-warn-targets=2"}, 0)
	checkPolicyExecutionFiles(t, directory, files)
}

func TestTargetLimitFlagsRejectInvalidValuesBeforeAnalysis(t *testing.T) {
	t.Parallel()

	root, directory, files := policyExecutionFixture(t, "not valid Go", targetLimitPolicy())
	for _, args := range [][]string{
		{"validate", "--warn-targets=0"},
		{"validate", "--strong-warn-targets=-1"},
		{"validate", "--max-targets=abc"},
		{"validate", "--warn-targets=4", "--strong-warn-targets=3"},
		{"scan", "--warn-targets=2"},
		{"version", "--max-targets=2"},
	} {
		reply := invokeTargetLimit(t, root, args, 2)
		if reply.Diagnostics[0].Code != model.CodeInvalidPolicy {
			t.Fatalf("invalid limit did not fail before analysis: %+v", reply.Diagnostics)
		}
	}

	checkPolicyExecutionFiles(t, directory, files)
}

func TestTargetLimitExplicitMaximum(t *testing.T) {
	t.Parallel()

	root, directory, files := policyExecutionFixture(t, targetLimitSource(), targetLimitPolicy())
	for _, args := range [][]string{
		{"validate", "--max-targets=2"},
		{"validate", "--max-targets=2", "--allow-large-plan"},
	} {
		reply := invokeTargetLimit(t, root, args, 5)
		if !strings.Contains(reply.Diagnostics[0].Message, "configured maximum") {
			t.Fatalf("configured maximum diagnostic missing: %+v", reply.Diagnostics)
		}
	}

	invokeTargetLimit(t, root, []string{"validate", "--max-targets=3"}, 0)
	invokeTargetLimit(t, root, []string{"validate", "--max-targets=3000", "--allow-large-plan"}, 0)
	checkPolicyExecutionFiles(t, directory, files)
}

func TestTargetLimitMaximumUsageErrors(t *testing.T) {
	t.Parallel()

	root, directory, files := policyExecutionFixture(t, "not valid Go", targetLimitPolicy())
	for _, value := range []string{"3000", "0", "-2", "999999999999999999999999", "private-value"} {
		reply := invokeTargetLimit(t, root, []string{"validate", "--max-targets=" + value}, 2)
		if reply.Diagnostics[0].Code != model.CodeInvalidPolicy {
			t.Fatalf("invalid usage diagnostic: %+v", reply.Diagnostics)
		}
	}

	checkPolicyExecutionFiles(t, directory, files)
}

func TestTargetLimitApplicability(t *testing.T) {
	t.Parallel()

	root, directory, files := policyExecutionFixture(t, "not valid Go", targetLimitPolicy())

	for _, command := range []string{"init", "help", "scan", "version"} {
		for _, flag := range []string{"--warn-targets=2", "--strong-warn-targets=2", "--max-targets=2"} {
			invokeTargetLimit(t, root, []string{command, flag}, 2)
		}
	}

	checkPolicyExecutionFiles(t, directory, files)
}

func TestTargetLimitDoesNotBypassOtherSafetyChecks(t *testing.T) {
	t.Parallel()

	checks := []struct {
		source string
		policy string
		code   model.Code
		exit   int
	}{
		{source: targetLimitSource(), policy: targetLimitPolicy() +
			"  attributes:\n  - key: password\n    from: {constant: caller-private-value}\n",
			code: model.CodeSecretAttribute, exit: 5},
		{source: "package app\nfunc RunA() {}\nfunc RunB() {}\nfunc RunC() {}\n",
			policy: targetLimitPolicy(), code: model.CodeMissingContext, exit: 5},
		{source: strings.Replace(targetLimitSource(), "package app", "package main", 1) + "func main() {}\n",
			policy: targetLimitPolicy(), code: model.CodeBackendUnsupported, exit: 7},
	}
	for _, check := range checks {
		root, directory, files := policyExecutionFixture(t, check.source, check.policy)
		args := []string{"validate", "--warn-targets=3000", "--strong-warn-targets=3000",
			"--max-targets=3000", "--allow-large-plan"}
		reply := policyExecutionReply(t.Context(), t, root, args, check.exit)

		found := false
		for _, diagnostic := range reply.Diagnostics {
			found = found || diagnostic.Code == check.code
		}

		if !found {
			t.Fatalf("safety check %s bypassed: %+v", check.code, reply.Diagnostics)
		}

		checkPolicyExecutionFiles(t, directory, files)
	}
}

func TestTargetLimitCheckDiffDoNotChangeLock(t *testing.T) {
	t.Parallel()

	root, files := lockContractFixture(t)

	directory, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := directory.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	})
	invokeTargetLimit(t, root, []string{"lock"}, 0)

	before := snapshotRootFiles(t, directory, files, "otelplan.lock")
	for _, args := range [][]string{
		{"lock", "--check", "--warn-targets=2", "--strong-warn-targets=3", "--max-targets=4"},
		{"diff", "--check", "--warn-targets=2", "--strong-warn-targets=3", "--max-targets=4"},
		{"lock", "--warn-targets=2", "--strong-warn-targets=3", "--max-targets=4"},
	} {
		invokeTargetLimit(t, root, args, 0)
		assertRootFilesEqual(t, directory, before)
	}
}

func invokeTargetLimit(t *testing.T, root string, args []string, want int) globalArgumentReply {
	t.Helper()

	arguments := append([]string{"--root", root, "--offline", "--format=json"}, args...)

	reply := runGlobalArgumentsJSON(t, arguments, want)
	if reply.Command != args[0] || reply.OK != (want == 0) {
		t.Fatalf("invalid threshold response: %+v", reply)
	}

	if want != 0 && len(reply.Diagnostics) == 0 {
		t.Fatal("failed threshold command has no diagnostic")
	}

	return reply
}

func targetLimitSource() string {
	return `package app
import "context"
func RunA(context.Context) error { return nil }
func RunB(context.Context) error { return nil }
func RunC(context.Context) error { return nil }
`
}

func targetLimitPolicy() string {
	return strings.Replace(executionPolicy, "functions: [Run]", "functions: [RunA, RunB, RunC]", 1)
}
