package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestSymbolMetadataCLIProvenanceAndDrift(t *testing.T) {
	t.Parallel()

	source := "package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n"
	policy := `apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
rules:
- id: metadata
  match: {functions: [Run]}
  span: {name: fixed}
  attributes:
  - key: code.package
    from: {symbol: packageName}
`
	root, directory, files := policyExecutionFixture(t, source, policy)
	original := testenv.SnapshotFiles(t, root)
	assertSymbolInspection(t, root)
	testenv.AssertFilesUnchanged(t, original, testenv.SnapshotFiles(t, root))
	runResolutionCommand(t, root, 0, "lock")
	runResolutionCommand(t, root, 0, "lock", "--check")

	locked, err := lockfile.Parse(readRootFile(t, directory, "otelplan.lock"))
	if err != nil {
		t.Fatal(err)
	}

	value := locked.Targets[0].Attributes[0].SymbolValue
	if value == nil || *value != "app" {
		t.Fatal("CLI lock did not freeze the selected metadata value")
	}

	assertSymbolValueRedaction(t, directory)

	err = directory.WriteFile("app.go", []byte(strings.Replace(source, "package app", "package renamed", 1)), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	before := snapshotRootFiles(t, directory, files, "otelplan.lock")
	for _, command := range [][]string{{"lock", "--check"}, {"diff", "--check"}} {
		runResolutionCommand(t, root, 6, command...)
		assertRootFilesEqual(t, directory, before)
	}
}

func assertSymbolValueRedaction(t *testing.T, directory *os.Root) {
	t.Helper()

	original := readRootFile(t, directory, "otelplan.lock")

	locked, err := lockfile.Parse(original)
	if err != nil {
		t.Fatal(err)
	}

	private := "caller-private-value"
	locked.Targets[0].Attributes[0].SymbolValue = &private

	encoded, err := lockfile.Marshal(locked)
	if err != nil {
		t.Fatal(err)
	}

	err = directory.WriteFile("otelplan.lock", encoded, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	before := testenv.SnapshotFiles(t, directory.Name())
	policyExecutionReply(t.Context(), t, directory.Name(), []string{"diff", "--check"}, 6)
	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, directory.Name()))

	err = directory.WriteFile("otelplan.lock", original, 0o600)
	if err != nil {
		t.Fatal(err)
	}
}

func assertSymbolInspection(t *testing.T, root string) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	args := []string{"inspect", "--root", root, "--offline", "--format=json"}

	exit := cli.Run(t.Context(), args, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("metadata inspection exit=%d: %s %s", exit, &stdout, &stderr)
	}

	var response struct {
		Data model.ResolvedPlan `json:"data"`
	}

	err := json.Unmarshal(stdout.Bytes(), &response)
	if err != nil {
		t.Fatal(err)
	}

	if len(response.Data.Targets) != 1 || len(response.Data.Targets[0].Attributes) != 1 {
		t.Fatalf("metadata inspection lost its target or explicit attributes: %s", &stdout)
	}

	attribute := response.Data.Targets[0].Attributes[0]
	if attribute.From.Symbol != "packageName" || attribute.From.Constant != nil {
		t.Fatal("metadata provenance was replaced with a constant or redacted")
	}

	stdout.Reset()
	stderr.Reset()

	args[len(args)-1] = "--format=text"

	exit = cli.Run(t.Context(), args, &stdout, &stderr)
	if exit != 0 || !strings.Contains(stdout.String(), "attribute code.package from symbol packageName") {
		t.Fatalf("text inspection omitted metadata provenance: exit=%d %s %s", exit, &stdout, &stderr)
	}
}
