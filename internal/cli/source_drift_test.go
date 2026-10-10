package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestLockCheckIgnoresSourceCoordinates(t *testing.T) {
	t.Parallel()

	const source = "package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n"

	tests := []struct {
		name           string
		source         string
		file           string
		classification model.DiffClassification
	}{
		{name: "line", source: strings.Replace(source, "func Run", "\n\nfunc Run", 1),
			file: "app.go", classification: ""},
		{name: "column", source: strings.Replace(source, "func Run", "\tfunc Run", 1),
			file: "app.go", classification: ""},
		{name: "line and column", source: strings.Replace(source, "func Run", "\n\n\tfunc Run", 1),
			file: "app.go", classification: ""},
		{name: "file", source: source, file: "moved.go", classification: model.DiffSource},
		{name: "file and coordinates", source: strings.Replace(source, "func Run", "\n\n\tfunc Run", 1),
			file: "moved.go", classification: model.DiffSource},
		{name: "signature", source: strings.Replace(source, "ctx context.Context", "ctx context.Context, value int", 1),
			file: "app.go", classification: model.DiffSignature},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root, directory, files := policyExecutionFixture(t, source, executionPolicy)
			runResolutionCommand(t, root, 0, "lock")

			err := directory.WriteFile("app.go", []byte(testCase.source), 0o600)
			if err != nil {
				t.Fatalf("change source fixture: %v", err)
			}

			if testCase.file != "app.go" {
				err = directory.Rename("app.go", testCase.file)
				if err != nil {
					t.Fatalf("move selected source file: %v", err)
				}
			}

			delete(files, "app.go")
			files[testCase.file] = testCase.source
			before := snapshotRootFiles(t, directory, files, "otelplan.lock")

			for _, command := range []string{"lock", "diff"} {
				if testCase.classification == "" {
					runResolutionCommand(t, root, 0, command, "--check")
				} else {
					assertSourceDrift(t, root, command, testCase.classification)
				}

				assertRootFilesEqual(t, directory, before)
			}
		})
	}
}

func assertSourceDrift(t *testing.T, root, command string, classification model.DiffClassification) {
	t.Helper()

	args := []string{command, "--check", "--root", root, "--offline", "--format=json"}
	reply := runGlobalArgumentsJSON(t, args, 6)

	if reply.Command != command || reply.OK {
		t.Fatalf("invalid source-drift response envelope: %+v", reply)
	}

	checkResolutionDiagnostics(t, reply, 6)

	var diff model.LockDiff

	err := json.Unmarshal(reply.Data, &diff)
	if err != nil {
		t.Fatalf("decode source-drift diff: %v", err)
	}

	if len(diff.Entries) != 1 || diff.Entries[0].Classification != classification ||
		diff.Entries[0].Symbol != "example.com/app.Run" || diff.Entries[0].Detail == "" {
		t.Fatalf("unexpected source drift: got %+v, want %s for example.com/app.Run", diff, classification)
	}
}
