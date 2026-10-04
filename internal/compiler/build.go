package compiler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

var (
	errUnverifiedBuildIdentities = errors.New("build requires verified module and backend identities")
	errChangedBuildBackend       = errors.New("build backend identity changed")
)

// PreparedBuildRequest binds a prepared workspace to its analyzed build settings and verified identities.
type PreparedBuildRequest struct {
	BuildEnvironment   model.BuildEnvironment
	Workspace          PreparedWorkspace
	ModuleDir          string
	Executable         string
	Backend            model.LockBackend
	ApplicationModules []model.ModuleInfo
	RuntimeModules     []model.ModuleInfo
	RuntimeOriginalDir string
	Env                []string
	GoArgs             []string
}

// BuildPrepared executes an already planned build. Env and GoArgs must match
// the analyzed build configuration; output paths must be resolved by the caller.
func BuildPrepared(ctx context.Context, request PreparedBuildRequest) error {
	err := validatePreparedBuildIdentities(request)
	if err != nil {
		return err
	}

	env, buildFlags, err := RecordedBuildEnvironment(request.Env, request.BuildEnvironment)
	if err != nil {
		return err
	}

	err = validatePreparedBuildWorkspace(request)
	if err != nil {
		return err
	}

	executable, err := verifyPreparedBuildBackend(ctx, request.Executable, request.Backend)
	if err != nil {
		return err
	}

	temporary, err := os.MkdirTemp(request.Workspace.Dir, "compiler-")
	if err != nil {
		return fmt.Errorf("create compiler temporary directory: %w", err)
	}

	defer func() { _ = os.RemoveAll(temporary) }()

	rules := filepath.Join(request.Workspace.Runtime.Dir, "rules")
	env = append(env, "GOWORK="+request.Workspace.WorkspaceFile, "GOTMPDIR="+temporary,
		"OTELC_RULES="+rules, "OTELC_WORK_DIR="+request.Workspace.Dir, "OTELC_BUILD_FLAGS=")

	err = verifyPreparedBuildSelection(ctx, request, env)
	if err != nil {
		return err
	}

	err = runPreparedBuildBackend(ctx, executable, request, env, buildFlags, rules)
	if err != nil {
		return err
	}

	err = VerifyArtifacts(request.Workspace.Runtime)
	if err != nil {
		return fmt.Errorf("backend changed runtime artifacts: %w", err)
	}

	return nil
}

func validatePreparedBuildIdentities(request PreparedBuildRequest) error {
	if len(request.ApplicationModules) == 0 || len(request.RuntimeModules) == 0 || request.Backend.Digest == "" {
		return errUnverifiedBuildIdentities
	}

	return ValidateBuildArguments(request.GoArgs, request.BuildEnvironment)
}

func validatePreparedBuildWorkspace(request PreparedBuildRequest) error {
	err := request.Workspace.validateBuildDirectory(request.ModuleDir)
	if err != nil {
		return err
	}

	return VerifyArtifacts(request.Workspace.Runtime)
}

func verifyPreparedBuildBackend(ctx context.Context, name string, expected model.LockBackend) (string, error) {
	executable, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("find build backend: %w", err)
	}

	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", fmt.Errorf("resolve build backend path: %w", err)
	}

	identity, err := otelc.VerifyExecutable(ctx, executable, expected.Version)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("verify build backend: %w", ctx.Err())
		}

		return "", fmt.Errorf("verify build backend: %w", err)
	}

	if identity != expected {
		return "", errChangedBuildBackend
	}

	return executable, nil
}

func verifyPreparedBuildSelection(ctx context.Context, request PreparedBuildRequest, env []string) error {
	err := verifyRecordedGoEnvironment(ctx, request.ModuleDir, env, request.BuildEnvironment)
	if err != nil {
		return err
	}

	selected, err := ReadModuleSelection(ctx, request.ModuleDir, env)
	if err != nil {
		return err
	}

	err = CheckModuleSelection(request.ApplicationModules, selected, request.Workspace.Relocations)
	if err != nil {
		return err
	}

	runtimeRelocations := map[string]string{request.RuntimeOriginalDir: request.Workspace.Runtime.Dir}

	return CheckModuleSelection(request.RuntimeModules, selected, runtimeRelocations)
}

func runPreparedBuildBackend(ctx context.Context, executable string, request PreparedBuildRequest,
	env, buildFlags []string, rules string) error {
	args := []string{"--rules", rules, "go", "build"}
	args = append(args, buildFlags...)

	if request.BuildEnvironment.ModuleMode == "vendor" {
		args = append(args, "-mod=vendor")
	}

	args = append(args, request.GoArgs...)
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir, command.Env = request.ModuleDir, env

	err := command.Run()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("backend build failed: %w", ctx.Err())
		}

		return fmt.Errorf("backend build failed: %w", err)
	}

	return nil
}
