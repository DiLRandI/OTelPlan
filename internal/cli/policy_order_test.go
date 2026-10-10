package cli_test

import (
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
)

func TestLockChecksIgnorePolicyRuleOrder(t *testing.T) {
	t.Parallel()

	header := "apiVersion: otelplan.io/v1alpha1\n" +
		"kind: InstrumentationPlan\n" +
		"backend: {name: otelc, version: v1.1.0}\n" +
		"rules:\n"
	first := "- id: operation\n  match:\n    functions: [Run]\n"
	second := "- id: other\n  match:\n    functions: [Other]\n"

	source := "package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n" +
		"\nfunc Other(ctx context.Context) error { return nil }\n"
	root, directory, files := policyExecutionFixture(t, source, header+first+second)

	runResolutionCommand(t, root, 0, "lock")

	locked, err := lockfile.Parse(readRootFile(t, directory, "otelplan.lock"))
	if err != nil {
		t.Fatalf("parse initial policy-order lock: %v", err)
	}

	if len(locked.Targets) != 2 {
		t.Fatalf("initial lock did not select both policy rules: %+v", locked.Targets)
	}

	wantRules := map[string]string{"example.com/app.Run": "operation", "example.com/app.Other": "other"}
	for _, target := range locked.Targets {
		if wantRules[string(target.Symbol)] != target.SourceRule {
			t.Fatalf("initial lock lost exact target provenance: %+v", target)
		}
	}

	reordered := header + second + first

	err = directory.WriteFile("otelplan.yaml", []byte(reordered), 0o600)
	if err != nil {
		t.Fatalf("reorder policy rules: %v", err)
	}

	beforeReordered := snapshotRootFiles(t, directory, files, "otelplan.lock")
	for _, args := range [][]string{{"lock", "--check"}, {"diff", "--check"}, {"lock"}} {
		runResolutionCommand(t, root, 0, args...)
		assertRootFilesEqual(t, directory, beforeReordered)
	}

	changed := strings.Replace(reordered, "- id: operation\n", "- id: revised\n", 1)

	err = directory.WriteFile("otelplan.yaml", []byte(changed), 0o600)
	if err != nil {
		t.Fatalf("change policy rule identity: %v", err)
	}

	beforeChanged := snapshotRootFiles(t, directory, files, "otelplan.lock")
	for _, args := range [][]string{{"lock", "--check"}, {"diff", "--check"}} {
		runResolutionCommand(t, root, 6, args...)
		assertRootFilesEqual(t, directory, beforeChanged)
	}
}
