package compiler_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
)

func TestPreparedBuildDryRunValidatesWithoutExecuting(t *testing.T) {
	t.Parallel()

	request := preparedBuildFixture(t)
	resetPreparedBackendVersionMarker(t, request)
	ready := filepath.Join(t.TempDir(), "ready")
	request.Env = append(request.Env, "OTELPLAN_TEST_MODE=fail", "OTELPLAN_TEST_READY="+ready)
	request.DryRun = true

	before := preparedWorkspaceEntries(t, request)

	err := compiler.BuildPrepared(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	assertPreparedBackendVersionVerified(t, request)
	assertPreparedPathMissing(t, ready, "dry-run executed backend build")
	assertPreparedWorkspaceUnchanged(t, request, before)

	err = compiler.VerifyArtifacts(request.Workspace.Runtime)
	if err != nil {
		t.Fatalf("dry-run changed runtime artifacts: %v", err)
	}
}

func TestPreparedBuildDryRunRetainsValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		change func(*compiler.PreparedBuildRequest)
		want   string
	}{
		{name: "missing application identity", change: func(request *compiler.PreparedBuildRequest) {
			request.ApplicationModules = nil
		}, want: "build requires verified module and backend identities"},
		{name: "invalid Go flag", change: func(request *compiler.PreparedBuildRequest) {
			request.GoArgs = []string{"-toolexec=caller-private-value"}
		}, want: "unsupported build flag"},
		{name: "changed backend digest", change: func(request *compiler.PreparedBuildRequest) {
			request.Backend.Digest = "sha256:" + strings.Repeat("0", 64)
		}, want: "build backend identity changed"},
		{name: "changed backend version", change: func(request *compiler.PreparedBuildRequest) {
			request.Backend.Version = "v9.9.9"
		}, want: "verify build backend"},
		{name: "changed module selection", change: func(request *compiler.PreparedBuildRequest) {
			request.ApplicationModules[0].Version = "v9.9.9"
		}, want: "build module selection changed"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			candidate := preparedBuildFixture(t)
			candidate.DryRun = true
			testCase.change(&candidate)

			err := compiler.BuildPrepared(t.Context(), candidate)

			invalid := err == nil || !strings.Contains(err.Error(), testCase.want) ||
				strings.Contains(err.Error(), "caller-private-value")
			if invalid {
				t.Fatalf("dry-run validation error=%v, want substring %q", err, testCase.want)
			}
		})
	}
}

func TestPreparedBuildDryRunRejectsAlteredRuntimeAndCancellation(t *testing.T) {
	t.Parallel()

	t.Run("altered runtime", func(t *testing.T) {
		t.Parallel()

		request := preparedBuildFixture(t)

		root, err := os.OpenRoot(request.Workspace.Runtime.Dir)
		if err != nil {
			t.Fatal(err)
		}

		err = root.WriteFile("manifest.json", []byte("changed"), 0o600)
		_ = root.Close()

		if err != nil {
			t.Fatal(err)
		}

		request.DryRun = true

		err = compiler.BuildPrepared(t.Context(), request)
		if err == nil {
			t.Fatal("dry-run accepted altered runtime artifacts")
		}
	})

	t.Run("canceled", func(t *testing.T) {
		t.Parallel()

		request := preparedBuildFixture(t)
		request.DryRun = true
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := compiler.BuildPrepared(ctx, request)
		if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "verify build backend") {
			t.Fatalf("dry-run cancellation error=%v", err)
		}
	})
}

func resetPreparedBackendVersionMarker(t *testing.T, request compiler.PreparedBuildRequest) {
	t.Helper()

	root, err := os.OpenRoot(filepath.Dir(request.Executable))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	err = root.Remove("version-ran")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}

func assertPreparedBackendVersionVerified(t *testing.T, request compiler.PreparedBuildRequest) {
	t.Helper()

	root, err := os.OpenRoot(filepath.Dir(request.Executable))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	_, err = root.Stat("version-ran")
	if err != nil {
		t.Fatalf("dry-run did not verify backend version: %v", err)
	}
}

func preparedWorkspaceEntries(t *testing.T, request compiler.PreparedBuildRequest) []fs.DirEntry {
	t.Helper()

	entries, err := os.ReadDir(request.Workspace.Dir)
	if err != nil {
		t.Fatal(err)
	}

	return entries
}

func assertPreparedWorkspaceUnchanged(t *testing.T, request compiler.PreparedBuildRequest, before []fs.DirEntry) {
	t.Helper()

	after := preparedWorkspaceEntries(t, request)
	if !slices.EqualFunc(before, after, func(a, b fs.DirEntry) bool { return a.Name() == b.Name() }) {
		t.Fatalf("dry-run left compiler temporary directories: before=%v after=%v", before, after)
	}
}

func assertPreparedPathMissing(t *testing.T, path, message string) {
	t.Helper()

	_, err := os.Stat(path)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s: %v", message, err)
	}
}
