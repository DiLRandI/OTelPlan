package otelc_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestStringerPanicWithPinnedBackendPreservesReturns(t *testing.T) {
	t.Parallel()

	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("set OTELPLAN_OTELC for real Stringer panic protection")
	}

	backend, err := otelc.VerifyExecutable(t.Context(), executable, otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	root := privateAccessorWorkspace(t)

	makeStringerPanic(t, root)
	original := snapshotApplicationFiles(t, root)
	code := discoverPrivateAccessors(t, root)
	plan := privateAccessorPlan(t, code)

	var attribute model.AttributePlan

	attribute.Key, attribute.From.Argument, attribute.Stringer = "display", "request", true
	plan.Targets[0].Attributes = []model.AttributePlan{attribute}

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
	if err != nil {
		t.Fatalf("Stringer probe failed: %v\n%s", err, output)
	}

	var result privateAccessorResult

	err = json.Unmarshal(output, &result)
	if err != nil {
		t.Fatal(err)
	}

	checkPrivateAccessorReturns(t, result)
	handles, downstream, parent := privateAccessorSpans(t, result.Spans)
	checkPrivateAccessorParents(t, handles, downstream, parent)

	events := 0

	for _, span := range handles {
		if len(span.Attributes) != 0 {
			t.Fatal("panicking or nil Stringer produced attributes")
		}

		events += span.Events
	}

	if events != 1 {
		t.Fatal("Stringer panic changed business-error events")
	}
}

func makeStringerPanic(t *testing.T, root *os.Root) {
	t.Helper()

	originalOps, err := root.ReadFile("ops/ops.go")
	if err != nil {
		t.Fatal(err)
	}

	panicking := strings.Replace(string(originalOps), `return "request:" + request.ID`, `panic("caller-private-value")`, 1)
	if panicking == string(originalOps) {
		t.Fatal("Stringer fixture replacement did not apply")
	}

	err = root.WriteFile("ops/ops.go", []byte(panicking), 0o600)
	if err != nil {
		t.Fatal(err)
	}
}
