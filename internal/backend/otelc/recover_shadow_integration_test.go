package otelc_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestPinnedBackendShadowedRecoverLimitation(t *testing.T) {
	t.Parallel()

	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("set OTELPLAN_OTELC for the pinned recover-shadowing regression")
	}

	backend, err := otelc.VerifyExecutable(t.Context(), executable, otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	root := privateAccessorWorkspace(t)

	// Render a safe bundle, then add the binding only to the disposable upstream probe.
	code := discoverPrivateAccessors(t, root)
	plan := privateAccessorPlan(t, code)
	plan.Targets[0].Attributes = nil

	files, err := otelc.RenderBundle(backend, "test", code, plan, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}

	ops, err := root.ReadFile("ops/ops.go")
	if err != nil {
		t.Fatal(err)
	}

	ops = append(ops, []byte("\nfunc recover() any { panic(\"shadowed builtin recover\") }\n")...)

	err = root.WriteFile("ops/ops.go", ops, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	original := snapshotApplicationFiles(t, root)
	assertShadowedRecoverRejected(t, root, backend)
	assertApplicationFilesUnchanged(t, root, original)
	writePrivateAccessorBundle(t, root, files, original)
	buildPrivateAccessorProbe(t, root, executable)
	assertApplicationFilesUnchanged(t, root, original)
	command := exec.CommandContext(t.Context(), "./probe")
	command.Dir = root.Name()

	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "shadowed builtin recover") ||
		!strings.Contains(string(output), "OtelBeforeTrampoline") {
		t.Fatalf("pinned trampoline limitation changed: %v\n%s", err, output)
	}
}

func assertShadowedRecoverRejected(t *testing.T, root *os.Root, backend model.LockBackend) {
	t.Helper()

	code := discoverPrivateAccessors(t, root)
	plan := privateAccessorPlan(t, code)
	plan.Targets[0].Attributes = nil

	diagnostics := otelc.Check(otelc.SupportedVersion, code, plan)
	if len(diagnostics) != 1 || diagnostics[0].Code != model.CodeBackendUnsupported ||
		diagnostics[0].Symbol != plan.Targets[0].SymbolID || diagnostics[0].RuleID != plan.Targets[0].RuleID {
		t.Fatalf("fresh analysis did not reject shadowed recover: %+v", diagnostics)
	}

	files, err := otelc.RenderBundle(backend, "test", code, plan, "example.com/generated")
	if err == nil || len(files) != 0 {
		t.Fatalf("render accepted shadowed recover: %v", err)
	}
}
