package compiler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

type ResolvedBuildRequest struct {
	Code                                           *model.CodeModel
	Plan                                           model.ResolvedPlan
	Backend                                        model.LockBackend
	Executable, RuntimeVersion, WorkingDir, Parent string
	Env, GoArgs                                    []string
	Offline                                        bool
	DefaultOutput                                  bool
	DirectoryOutput                                bool
	Packages                                       []string
}

type BuildResult struct {
	Dir   string
	Files []BuildArtifact
}

type BuildArtifact struct {
	Dir         string
	File        string
	Digest      string
	DefaultName string
}

// BuildResolved builds an already validated plan in disposable module copies.
// GoArgs must match analysis and omit -o. The caller owns returned Dir cleanup.
func BuildResolved(ctx context.Context, request ResolvedBuildRequest) (BuildResult, error) {
	if request.DefaultOutput && request.DirectoryOutput {
		return BuildResult{}, errors.New("build output modes are mutually exclusive")
	}

	if request.Code == nil {
		return BuildResult{}, errors.New("build requires analysis")
	}

	if request.Code.EffectiveBuild.ModuleMode == "vendor" {
		return BuildResult{}, errors.New("vendored builds require isolated vendor materialization")
	}

	if err := ValidateBuildArguments(request.GoArgs, request.Code.EffectiveBuild); err != nil {
		return BuildResult{}, err
	}

	for _, arg := range request.GoArgs {
		name, _, _ := strings.Cut(arg, "=")
		if name == "-o" || name == "--o" {
			return BuildResult{}, errors.New("resolved build owns its temporary output path")
		}
	}

	if request.Parent != "" {
		parent, err := filepath.Abs(request.Parent)
		if err != nil {
			return BuildResult{}, err
		}

		request.Parent = parent
	}

	workspaceRequest, err := WorkspaceForAnalysis(request.Code)
	if err != nil {
		return BuildResult{}, err
	}

	files, err := otelc.RenderBundle(request.Backend, request.RuntimeVersion, request.Code, request.Plan, "otelplan.local/generated")
	if err != nil {
		return BuildResult{}, err
	}

	runtime, err := StageArtifacts(request.Parent, files)
	if err != nil {
		return BuildResult{}, err
	}

	defer func() { _ = os.RemoveAll(runtime.Dir) }()

	workspaceRequest.Runtime, workspaceRequest.Parent = runtime, request.Parent

	prepared, err := PrepareWorkspace(ctx, workspaceRequest)
	if err != nil {
		return BuildResult{}, err
	}

	complete := false

	defer func() {
		if !complete {
			_ = os.RemoveAll(prepared.Dir)
		}
	}()

	workingDir := request.WorkingDir
	if workingDir == "" {
		workingDir = request.Code.ModuleRoot
	}

	copiedDir, err := prepared.BuildDirectory(workingDir)
	if err != nil && request.Code.EffectiveBuild.Workspace && filepath.Clean(workingDir) == filepath.Dir(request.Code.WorkspaceFile) {
		if buildPackageStart(request.GoArgs) >= len(request.GoArgs) {
			return BuildResult{}, errors.New("workspace-root build requires explicit package targets")
		}

		copiedDir, err = prepared.applicationBuildDirectory()
	}

	if err != nil {
		return BuildResult{}, err
	}

	env, buildFlags, err := RecordedBuildEnvironment(request.Env, request.Code.EffectiveBuild)
	if err != nil {
		return BuildResult{}, err
	}

	if request.Offline {
		env = append(env, "GOPROXY=off", "GONOPROXY=none", "GOSUMDB=off")
	}

	runtimeSelection, err := ReadModuleSelection(ctx, runtime.Dir, append(append([]string(nil), env...), "GOWORK=off"))
	if err != nil {
		return BuildResult{}, err
	}

	applicationModules, err := applicationModuleSelection(ctx, prepared, copiedDir, env)
	if err != nil {
		return BuildResult{}, err
	}

	if err := CheckModuleSelection(request.Code.Modules, applicationModules, prepared.Relocations); err != nil {
		return BuildResult{}, err
	}

	output := filepath.Join(prepared.Dir, "output")

	relocatedArgs, err := prepared.RelocateBuildArguments(request.GoArgs, workingDir)
	if err != nil {
		return BuildResult{}, err
	}

	defaultName := ""
	discard := false

	if request.DefaultOutput {
		targets, err := prepared.RelocateBuildArguments(request.Packages, workingDir)
		if err != nil {
			return BuildResult{}, err
		}

		defaultName, err = defaultBuildOutput(ctx, copiedDir, append(append([]string(nil), env...), "GOWORK="+prepared.WorkspaceFile), buildFlags, targets, request.Code.EffectiveBuild.GOOS)
		if err != nil {
			return BuildResult{}, err
		}

		discard = defaultName == ""
	}

	if request.DirectoryOutput {
		err := os.Mkdir(output, 0o700)
		if err != nil {
			return BuildResult{}, err
		}
	}

	args := relocatedArgs
	if !discard {
		args = append([]string{"-o", output}, relocatedArgs...)
	}

	err = BuildPrepared(ctx, PreparedBuildRequest{BuildEnvironment: request.Code.EffectiveBuild, Workspace: prepared, ModuleDir: copiedDir, Executable: request.Executable, Backend: request.Backend, ApplicationModules: applicationModules, RuntimeModules: runtimeSelection, RuntimeOriginalDir: runtime.Dir, Env: env, GoArgs: args})
	if err != nil {
		return BuildResult{}, err
	}

	result := BuildResult{Dir: prepared.Dir}

	if !discard {
		paths := []string{output}

		if request.DirectoryOutput {
			entries, err := os.ReadDir(output)
			if err != nil {
				return BuildResult{}, err
			}

			paths = nil
			for _, entry := range entries {
				paths = append(paths, filepath.Join(output, entry.Name()))
			}
		}

		for _, path := range paths {
			name := defaultName
			if request.DirectoryOutput {
				name = filepath.Base(path)
			}

			artifact, err := readBuildArtifact(prepared.Dir, path, name)
			if err != nil {
				return BuildResult{}, err
			}

			result.Files = append(result.Files, artifact)
		}
	}

	complete = true

	return result, nil
}
