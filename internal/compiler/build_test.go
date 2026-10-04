package compiler_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const preparedBuildFailureCode = 7

func TestPreparedBuildRequiresVerifiedIdentities(t *testing.T) {
	t.Parallel()

	for _, missing := range []string{"application modules", "runtime modules", "backend digest"} {
		t.Run(missing, func(t *testing.T) {
			t.Parallel()

			var request compiler.PreparedBuildRequest

			var module model.ModuleInfo

			module.Path = "example.com/app"
			request.ApplicationModules, request.RuntimeModules = []model.ModuleInfo{module}, []model.ModuleInfo{module}
			request.Backend.Digest = fmt.Sprintf("sha256:%x", sha256.Sum256(nil))

			switch missing {
			case "application modules":
				request.ApplicationModules = nil
			case "runtime modules":
				request.RuntimeModules = nil
			case "backend digest":
				request.Backend.Digest = ""
			}

			first := compiler.BuildPrepared(t.Context(), request)

			second := compiler.BuildPrepared(t.Context(), request)
			if first == nil || !errors.Is(second, first) ||
				first.Error() != "build requires verified module and backend identities" {
				t.Fatal("missing build identities lost their stable cause or meaningful message", first, second)
			}
		})
	}
}

func preparedBuildExecutable(t *testing.T) string {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	source := fmt.Sprintf(`package main
import ("fmt"; "os"; "time")
func main() {
 if len(os.Args) > 1 && os.Args[1] == "version" { fmt.Print("otelc version v1.1.0"); return }
 if os.Getenv("OTELPLAN_TEST_MODE") == "fail" { os.Exit(%d) }
 if err := os.WriteFile(os.Getenv("OTELPLAN_TEST_READY"), nil, 0600); err != nil { os.Exit(1) }
 time.Sleep(time.Hour)
}
`, preparedBuildFailureCode)

	err = root.WriteFile("backend.go", []byte(source), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	executable := filepath.Join(root.Name(), "backend.exe")
	command := exec.CommandContext(t.Context(), "go", "build", "-o", "backend.exe", "backend.go")
	command.Dir = root.Name()

	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOPROXY=off")

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build test backend: %v\n%s", err, output)
	}

	return executable
}

func preparedBuildFixture(t *testing.T) compiler.PreparedBuildRequest {
	t.Helper()

	fixture := newWorkspacePreparationFixture(t)
	files := []otelc.GeneratedFile{{Path: "go.mod", Data: []byte("module example.com/runtime\n\ngo 1.25.0\n")}}

	runtime, err := compiler.StageArtifacts(t.TempDir(), files)
	if err != nil {
		t.Fatal(err)
	}

	fixture.request.Runtime = runtime
	fixture.request.WorkspaceSums = nil

	var options discovery.Options

	options.Root = fixture.request.OriginalWorkspaceDir
	options.Env = []string{"GOWORK=off", "GOFLAGS="}
	options.Offline = true

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := compiler.PrepareWorkspace(t.Context(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOPROXY=off")

	selection, err := compiler.ReadModuleSelection(t.Context(), runtime.Dir, env)
	if err != nil {
		t.Fatal(err)
	}

	executable := preparedBuildExecutable(t)

	backend, err := otelc.VerifyExecutable(t.Context(), executable, otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	return compiler.PreparedBuildRequest{
		BuildEnvironment: code.EffectiveBuild, Workspace: prepared,
		ModuleDir: prepared.Relocations[fixture.request.OriginalWorkspaceDir], Executable: executable, Backend: backend,
		ApplicationModules: code.Modules, RuntimeModules: selection, RuntimeOriginalDir: runtime.Dir, Env: env, GoArgs: nil,
	}
}

func TestPreparedBuildBackendIdentitySharesCause(t *testing.T) {
	t.Parallel()

	request := preparedBuildFixture(t)
	request.Backend.Digest = fmt.Sprintf("sha256:%x", sha256.Sum256(nil))
	first := compiler.BuildPrepared(t.Context(), request)

	second := compiler.BuildPrepared(t.Context(), request)
	if first == nil || !errors.Is(second, first) || first.Error() != "build backend identity changed" {
		t.Fatal("backend mismatch lost its stable cause or meaningful message", first, second)
	}
}

func TestPreparedBuildPreservesBackendExitCause(t *testing.T) {
	t.Parallel()

	request := preparedBuildFixture(t)
	request.Env = append(request.Env, "OTELPLAN_TEST_MODE=fail")

	err := compiler.BuildPrepared(t.Context(), request)

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != preparedBuildFailureCode ||
		!strings.Contains(err.Error(), "backend build failed") {
		t.Fatal("backend failure lost its subprocess cause or operation", err)
	}
}

func TestPreparedBuildCancellationPreservesVerificationCause(t *testing.T) {
	t.Parallel()

	request := preparedBuildFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := compiler.BuildPrepared(ctx, request)
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "verify build backend") {
		t.Fatal("cancelled backend verification lost its cause or operation", err)
	}
}

func TestPreparedBuildCancellationStopsBackend(t *testing.T) {
	t.Parallel()

	request := preparedBuildFixture(t)
	marker := filepath.Join(t.TempDir(), "ready")
	request.Env = append(request.Env, "OTELPLAN_TEST_READY="+marker)

	before, err := os.ReadDir(request.Workspace.Dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	completed := make(chan error, 1)

	go func() { completed <- compiler.BuildPrepared(ctx, request) }()

	consumed := false

	t.Cleanup(func() {
		cancel()

		if !consumed {
			<-completed
		}
	})
	waitForPreparedBuildStart(t, marker, completed, &consumed)
	cancel()

	select {
	case err := <-completed:
		consumed = true

		if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "backend build failed") {
			t.Fatal("backend cancellation lost its cause or operation", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancelled backend did not exit")
	}

	after, err := os.ReadDir(request.Workspace.Dir)
	if err != nil || !slices.EqualFunc(before, after, func(a, b fs.DirEntry) bool { return a.Name() == b.Name() }) {
		t.Fatal("cancelled build left temporary directories", err)
	}

	err = compiler.VerifyArtifacts(request.Workspace.Runtime)
	if err != nil {
		t.Fatal("cancelled build changed runtime artifacts", err)
	}
}

func waitForPreparedBuildStart(t *testing.T, marker string, completed <-chan error, consumed *bool) {
	t.Helper()

	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-completed:
			*consumed = true

			t.Fatal("backend stopped before cancellation", err)
		case <-deadline.C:
			t.Fatal("backend did not start")
		case <-ticker.C:
			_, err := os.Stat(marker)
			if err == nil {
				return
			}

			if !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("inspect backend readiness", err)
			}
		}
	}
}
