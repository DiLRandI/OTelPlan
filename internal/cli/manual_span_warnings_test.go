package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestManualSpanWarningPreservesSelectionAndPrivacy(t *testing.T) {
	t.Setenv("GOOS", runtime.GOOS)
	t.Setenv("GOARCH", runtime.GOARCH)
	testenv.PrepareOffline(t, "testdata/architectures/common")
	root := manualSpanCLIFixture(t)
	original := testenv.SnapshotFiles(t, root.Name())

	for _, strict := range []bool{false, true} {
		args := []string{"inspect", "--root", root.Name(), "--offline", "--format=json"}
		wantExit := 0

		if strict {
			args = append(args, "--strict")
			wantExit = 5
		}

		var stdout, stderr bytes.Buffer

		exit := cli.Run(t.Context(), args, &stdout, &stderr)
		if exit != wantExit {
			t.Fatalf("strict=%v: exit=%d want=%d: %s %s", strict, exit, wantExit, &stdout, &stderr)
		}

		assertManualSpanCLIResponse(t, stdout.Bytes(), !strict)
	}

	var stdout, stderr bytes.Buffer

	args := []string{"validate", "--root", root.Name(), "--offline"}

	exit := cli.Run(t.Context(), args, &stdout, &stderr)
	if exit != 0 || !strings.Contains(stdout.String(), "OTP4007") {
		t.Fatalf("text manual-span diagnostic missing: exit=%d %s %s", exit, &stdout, &stderr)
	}

	if strings.Contains(stdout.String()+stderr.String(), "private-span-label-do-not-log") {
		t.Fatal("text diagnostic exposed a span-name value")
	}

	testenv.AssertFilesUnchanged(t, original, testenv.SnapshotFiles(t, root.Name()))
}

func assertManualSpanCLIResponse(t *testing.T, output []byte, wantOK bool) {
	t.Helper()

	var response struct {
		OK          bool                      `json:"ok"`
		Diagnostics model.DiagnosticErrorList `json:"diagnostics"`
		Data        model.ResolvedPlan        `json:"data"`
	}

	err := json.Unmarshal(output, &response)
	if err != nil {
		t.Fatal(err)
	}

	if response.OK != wantOK || len(response.Diagnostics) != 1 || len(response.Data.Targets) != 1 {
		t.Fatalf("unexpected manual-span response: %s", output)
	}

	warning := response.Diagnostics[0]
	assertManualSpanWarningContext(t, warning, response.Data.Targets[0])

	if strings.Contains(string(output), "private-span-label-do-not-log") {
		t.Fatal("JSON diagnostic exposed a span-name value")
	}
}

func assertManualSpanWarningContext(t *testing.T, warning model.DiagnosticError, selected model.ResolvedTarget) {
	t.Helper()

	if warning.Code != model.CodeManualSpan || warning.Severity != model.SeverityWarning ||
		warning.RuleID != "manual-operation" || warning.Symbol != "example.com/manual.Run" ||
		selected.SymbolID != warning.Symbol {
		t.Fatalf("manual-span warning changed exact selection or lost context: %+v", warning)
	}
}

func manualSpanCLIFixture(t *testing.T) *os.Root {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	source, err := os.OpenRoot("testdata/architectures/common")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = source.Close() }()

	for _, name := range []string{"go.mod", "go.sum"} {
		contents, readErr := source.ReadFile(name)
		if readErr != nil {
			t.Fatal(readErr)
		}

		if name == "go.mod" {
			module := strings.Replace(string(contents), "module example.com/architecture", "module example.com/manual", 1)
			contents = []byte(module)
		}

		err = root.WriteFile(name, contents, 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	for name, contents := range map[string]string{
		"app.go": `package manual
import (
 "context"
 "go.opentelemetry.io/otel/trace"
)
func Run(ctx context.Context, tracer trace.Tracer) error {
 _, span := tracer.Start(ctx, "private-span-label-do-not-log")
 defer span.End()
 return nil
}
`,
		"otelplan.yaml": `apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
rules:
- id: manual-operation
  match: {functions: [Run]}
`,
	} {
		err = root.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	return root
}
