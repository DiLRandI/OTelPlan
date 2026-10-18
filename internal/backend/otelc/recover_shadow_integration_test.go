package otelc_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
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
	code := discoverPrivateAccessors(t, root)
	plan := privateAccessorPlan(t, code)
	plan.Targets[0].Attributes = nil

	files, err := otelc.RenderBundle(backend, "test", code, plan, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}

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
