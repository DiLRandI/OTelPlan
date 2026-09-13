package compiler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

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
	if len(request.ApplicationModules) == 0 || len(request.RuntimeModules) == 0 || request.Backend.Digest == "" {
		return fmt.Errorf("build requires verified module and backend identities")
	}
	baseEnv, buildFlags, err := RecordedBuildEnvironment(request.Env, request.BuildEnvironment)
	if err != nil {
		return err
	}
	known := false
	for _, dir := range request.Workspace.Relocations {
		if dir == request.ModuleDir && withinTree(request.Workspace.Dir, dir) {
			known = true
		}
	}
	if !known || !filepath.IsAbs(request.ModuleDir) {
		return fmt.Errorf("build directory is not a prepared application module")
	}
	if err := VerifyArtifacts(request.Workspace.Runtime); err != nil {
		return err
	}
	executable, err := exec.LookPath(request.Executable)
	if err != nil {
		return fmt.Errorf("find build backend: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return err
	}
	identity, err := otelc.VerifyExecutable(ctx, executable, request.Backend.Version)
	if err != nil {
		return err
	}
	if identity != request.Backend {
		return fmt.Errorf("build backend identity changed")
	}
	temporary, err := os.MkdirTemp(request.Workspace.Dir, "compiler-")
	if err != nil {
		return fmt.Errorf("create compiler temporary directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	rules := filepath.Join(request.Workspace.Runtime.Dir, "rules")
	env := append(baseEnv, "GOWORK="+request.Workspace.WorkspaceFile, "GOTMPDIR="+temporary, "OTELC_RULES="+rules, "OTELC_WORK_DIR="+request.Workspace.Dir, "OTELC_BUILD_FLAGS=")
	if err := verifyRecordedGoEnvironment(ctx, request.ModuleDir, env, request.BuildEnvironment); err != nil {
		return err
	}
	selected, err := ReadModuleSelection(ctx, request.ModuleDir, env)
	if err != nil {
		return err
	}
	if err := CheckModuleSelection(request.ApplicationModules, selected, request.Workspace.Relocations); err != nil {
		return err
	}
	if err := CheckModuleSelection(request.RuntimeModules, selected, map[string]string{request.RuntimeOriginalDir: request.Workspace.Runtime.Dir}); err != nil {
		return err
	}
	args := append([]string{"--rules", rules, "go", "build"}, buildFlags...)
	args = append(args, request.GoArgs...)
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = request.ModuleDir
	command.Env = env
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("backend build failed: %w", err)
	}
	if err := VerifyArtifacts(request.Workspace.Runtime); err != nil {
		return fmt.Errorf("backend changed runtime artifacts: %w", err)
	}
	return nil
}
