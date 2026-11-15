package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/testenv"
)

func TestCLICompilerFingerprintRelocationAndDrift(t *testing.T) {
	t.Setenv("GOENV", "off")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("CGO_ENABLED", "1")

	const source = "package app\nimport \"context\"\nfunc Run(context.Context) error { return nil }\n"

	first, firstRoot, _ := policyExecutionFixture(t, source, executionPolicy)
	moved, movedRoot, _ := policyExecutionFixture(t, source, executionPolicy)
	compiler, changed := cgoFingerprintCompiler(t, first), cgoFingerprintCompiler(t, moved)
	t.Setenv("CC", compiler)
	t.Setenv("CXX", compiler)

	invoke(t, first, 0, "lock")
	locked := readLockTestFile(t, firstRoot, "otelplan.lock")
	writeLockTestFile(t, movedRoot, "otelplan.lock", string(locked))
	before := testenv.SnapshotFiles(t, moved)

	invoke(t, moved, 0, "lock", "--check")

	t.Setenv("CC", changed)
	invoke(t, moved, 6, "lock", "--check")

	t.Setenv("CC", compiler)
	t.Setenv("CXX", changed)
	invoke(t, moved, 6, "lock", "--check")

	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, moved))
}

func cgoFingerprintCompiler(t *testing.T, project string) string {
	t.Helper()

	directory := project + "-tools"
	requireTestSuccess(t, os.Mkdir(directory, 0o700), "create compiler metadata probe")
	t.Cleanup(func() {
		err := os.RemoveAll(directory)
		if err != nil {
			t.Errorf("remove compiler metadata probe: %v", err)
		}
	})

	root := openLockTestRoot(t, directory)
	writeLockTestFile(t, root, "compiler", "#!/bin/sh\nexit 0\n")
	requireTestSuccess(t, root.Chmod("compiler", 0o700), "make compiler metadata probe executable")

	return filepath.Join(directory, "compiler")
}
