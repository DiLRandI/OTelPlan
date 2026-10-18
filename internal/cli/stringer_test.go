package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestStringerCLIProvenanceAndLockDrift(t *testing.T) {
	t.Parallel()

	source := `package app
import "context"
type Category string
func (Category) String() string { panic("inspection must not execute methods") }
func Run(ctx context.Context, value Category) error { return nil }
`
	policy := executionPolicy + "  attributes:\n  - key: category\n    stringer: true\n    from: {argument: value}\n"
	root, directory, files := policyExecutionFixture(t, source, policy)
	before := testenv.SnapshotFiles(t, root)
	reply := runGlobalArgumentsJSON(t, []string{"inspect", "--root", root, "--offline", "--format=json"}, 0)

	var plan model.ResolvedPlan

	err := json.Unmarshal(reply.Data, &plan)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Targets) != 1 || len(plan.Targets[0].Attributes) != 1 || !plan.Targets[0].Attributes[0].Stringer {
		t.Fatal("Stringer provenance lost")
	}

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), []string{"inspect", "--root", root, "--offline"}, &stdout, &stderr)
	if exit != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "argument value stringer=true") {
		t.Fatal("text inspection lost conversion intent")
	}

	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))
	runResolutionCommand(t, root, 0, "lock")
	runResolutionCommand(t, root, 0, "lock", "--check")

	changed := strings.Replace(policy, "stringer: true", "stringer: false", 1)

	err = directory.WriteFile("otelplan.yaml", []byte(changed), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	locked := snapshotRootFiles(t, directory, files, "otelplan.lock")
	for _, args := range [][]string{{"lock", "--check"}, {"diff", "--check"}} {
		runResolutionCommand(t, root, 6, args...)
		assertRootFilesEqual(t, directory, locked)
	}
}
