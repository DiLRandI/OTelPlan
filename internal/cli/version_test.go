package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestVersionBackendWithoutExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := t.TempDir()

	reply, versions := runVersionJSON(t.Context(), t, root, nil, 0)
	if !reply.OK || versions["otelplan"] == "" || versions["go"] != runtime.Version() ||
		versions["backend"] != "otelc" || versions["backendSupportedVersion"] != "v1.1.0" ||
		versions["backendStatus"] != "unavailable" {
		t.Fatalf("standalone backend details missing: %v", versions)
	}

	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("version changed standalone directory: %v, %v", entries, err)
	}
}

func TestVersionConfiguredBackendWithoutAnalysis(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root, directory, files := policyExecutionFixture(t, "not valid Go", `apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.2.0}
rules: []
`)

	_, versions := runVersionJSON(t.Context(), t, root, nil, 0)
	if versions["backendConfiguredVersion"] != "v1.2.0" || versions["backendStatus"] != "unavailable" {
		t.Fatalf("configured backend missing: %v", versions)
	}

	_, versions = runVersionJSON(t.Context(), t, root, []string{"--config", filepath.Join(root, "otelplan.yaml")}, 0)
	if versions["backendConfiguredVersion"] != "v1.2.0" {
		t.Fatalf("explicit configured backend missing: %v", versions)
	}

	checkPolicyExecutionFiles(t, directory, files)
}

func TestVersionInvalidConfigurationRedactsValues(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	for _, contents := range []string{"backend: [caller-private-value",
		"backend: {name: caller-private-value, version: v1.1.0}",
		"backend: {name: otelc, version: caller-private-value}"} {
		root, directory, files := policyExecutionFixture(t, "not valid Go", contents)
		reply, versions := runVersionJSON(t.Context(), t, root, []string{"--verbose"}, 3)
		checkPolicyDiagnosticCodes(t, reply, []model.Code{model.CodeInvalidPolicy})

		if versions["otelplan"] == "" || versions["go"] == "" || versions["backendStatus"] != "unverified" {
			t.Fatal("configuration failure lost tool identity or misreported probe status")
		}

		checkPolicyExecutionFiles(t, directory, files)
	}

	reply, _ := runVersionJSON(t.Context(), t, t.TempDir(), []string{"--config=missing.yaml"}, 3)
	checkPolicyDiagnosticCodes(t, reply, []model.Code{model.CodeInvalidPolicy})
}

func TestVersionCanceledContext(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	reply, versions := runVersionJSON(ctx, t, t.TempDir(), []string{"--verbose"}, 7)
	checkPolicyDiagnosticCodes(t, reply, []model.Code{model.CodeBackendUnsupported})

	if versions["backendStatus"] != "unverified" {
		t.Fatalf("cancellation status=%v", versions)
	}
}

func runVersionJSON(ctx context.Context, t *testing.T, root string, extra []string, wantExit int,
) (globalArgumentReply, map[string]string) {
	t.Helper()

	args := append([]string{"version", "--root", root, "--format=json"}, extra...)

	var stdout, stderr bytes.Buffer

	exit := cli.Run(ctx, args, &stdout, &stderr)
	if exit != wantExit || stderr.Len() != 0 || strings.Contains(stdout.String(), "caller-private-value") {
		t.Fatalf("version exit=%d want=%d stdout=%s stderr=%s", exit, wantExit, &stdout, &stderr)
	}

	var reply globalArgumentReply

	err := json.Unmarshal(stdout.Bytes(), &reply)
	if err != nil || reply.Command != "version" || reply.APIVersion != cli.APIVersion || reply.OK != (wantExit == 0) {
		t.Fatalf("invalid version envelope: %v, %s", err, &stdout)
	}

	var versions map[string]string

	err = json.Unmarshal(reply.Data, &versions)
	if err != nil {
		t.Fatalf("invalid version data: %v", err)
	}

	return reply, versions
}
