package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestRecursiveTargetWarningAndStrictValidation(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	for name, contents := range map[string]string{
		"go.mod": "module example.com/recursion\n\ngo 1.27\n",
		"app.go": `package recursion
import "context"
func Run(ctx context.Context, count int) error {
 if count > 0 { return Run(ctx, count-1) }
 return nil
}
`,
		"otelplan.yaml": `apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
rules:
- id: recursive-operation
  match: {functions: [Run]}
`,
	} {
		err = root.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	original := testenv.SnapshotFiles(t, root.Name())

	for _, strict := range []bool{false, true} {
		args := []string{"validate", "--root", root.Name(), "--offline", "--format=json"}
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

		assertRecursiveValidationJSON(t, stdout.Bytes(), !strict)
	}

	testenv.AssertFilesUnchanged(t, original, testenv.SnapshotFiles(t, root.Name()))
}

func assertRecursiveValidationJSON(t *testing.T, output []byte, wantOK bool) {
	t.Helper()

	var response struct {
		OK          bool                      `json:"ok"`
		Diagnostics model.DiagnosticErrorList `json:"diagnostics"`
	}

	err := json.Unmarshal(output, &response)
	if err != nil {
		t.Fatal(err)
	}

	if response.OK != wantOK || len(response.Diagnostics) != 1 {
		t.Fatalf("unexpected recursive validation response: %s", output)
	}

	warning := response.Diagnostics[0]
	if warning.Code != model.CodeRecursiveTarget || warning.Severity != model.SeverityWarning ||
		warning.RuleID != "recursive-operation" || warning.Symbol != "example.com/recursion.Run" {
		t.Fatalf("recursive warning lost context: %+v", warning)
	}
}
