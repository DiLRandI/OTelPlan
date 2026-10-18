package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestErrorTypeCLIInspectionAndLockDrift(t *testing.T) {
	t.Parallel()

	source := `package app
import ("context"; "errors")
func Run(context.Context) (first error, second error) { return errors.New("caller-private-value"), nil }
`
	policy := `apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
rules:
- id: classify
  match: {functions: [Run]}
  errors: {record: false}
  attributes:
  - key: failure.class
    from: {errorType: first}
`
	root, directory, files := policyExecutionFixture(t, source, policy)
	before := testenv.SnapshotFiles(t, root)
	reply := runGlobalArgumentsJSON(t, []string{"inspect", "--root", root, "--offline", "--format=json"}, 0)

	assertErrorTypePlan(t, reply.Data)

	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))
	runResolutionCommand(t, root, 0, "lock")

	locked, err := lockfile.Parse(readRootFile(t, directory, "otelplan.lock"))
	if err != nil {
		t.Fatal(err)
	}

	attribute := locked.Targets[0].Attributes[0]
	if attribute.From.ErrorType != "first" || attribute.Kind != "string" || attribute.SymbolValue != nil {
		t.Fatal("error type lock source changed")
	}

	original := snapshotRootFiles(t, directory, files, "otelplan.lock")
	for _, args := range [][]string{{"lock"}, {"lock", "--check"}, {"diff", "--check"}} {
		runResolutionCommand(t, root, 0, args...)
		assertRootFilesEqual(t, directory, original)
	}

	changed := strings.Replace(policy, "errorType: first", "errorType: second", 1)

	err = directory.WriteFile("otelplan.yaml", []byte(changed), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	beforeCheck := snapshotRootFiles(t, directory, files, "otelplan.lock")
	for _, args := range [][]string{{"lock", "--check"}, {"diff", "--check"}} {
		runResolutionCommand(t, root, 6, args...)
		assertRootFilesEqual(t, directory, beforeCheck)
	}

	if bytes.Contains(readRootFile(t, directory, "otelplan.lock"), []byte("caller-private-value")) {
		t.Fatal("lock contained an application error message")
	}
}

func assertErrorTypePlan(t *testing.T, data json.RawMessage) {
	t.Helper()

	var plan model.ResolvedPlan

	err := json.Unmarshal(data, &plan)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Targets) != 1 || len(plan.Targets[0].Attributes) != 1 {
		t.Fatal("error classification target missing")
	}

	if plan.Targets[0].Attributes[0].From.ErrorType != "first" || plan.Targets[0].ErrorStrategy.Record {
		t.Fatal("classification lost its explicit source or enabled error recording")
	}
}

func TestErrorTypeCLITextInspection(t *testing.T) {
	t.Parallel()

	source := "package app\nimport \"context\"\nfunc Run(context.Context)(err error) { return nil }\n"
	policy := executionPolicy + "  attributes:\n  - key: failure.class\n    from: {errorType: err}\n"
	root, directory, files := policyExecutionFixture(t, source, policy)

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), []string{"inspect", "--root", root, "--offline"}, &stdout, &stderr)
	if exit != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "attribute failure.class from errorType err") {
		t.Fatalf("error-type text provenance missing: exit=%d stdout=%s stderr=%s", exit, &stdout, &stderr)
	}

	checkPolicyExecutionFiles(t, directory, files)
}
