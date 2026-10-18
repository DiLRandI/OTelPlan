package compiler

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

var (
	errConflictingResolvedBuildOutputs  = errors.New("build output modes are mutually exclusive")
	errResolvedBuildWithoutAnalysis     = errors.New("build requires analysis")
	errResolvedBuildOutputArgument      = errors.New("resolved build owns its temporary output path")
	errWorkspaceRootWithoutBuildTargets = errors.New("workspace-root build requires explicit package targets")
)

// ResolvedBuildRequest combines an analyzed plan, pinned backend, and output choices.
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
	DryRun                                         bool
}

// BuildResult owns the disposable workspace. Files contains verified artifacts
// after execution. Dry-runs instead populate PreviewNames with planned default
// filenames; an empty name means the caller supplies the output filename.
type BuildResult struct {
	Dir          string
	Files        []BuildArtifact
	PreviewNames []string
}

// BuildArtifact identifies a temporary output by its digest and optional default filename.
type BuildArtifact struct {
	Dir         string
	File        string
	Digest      string
	DefaultName string
}

type resolvedBuildOutput struct {
	path         string
	defaultName  string
	discard      bool
	arguments    []string
	previewNames []string
}

// BuildResolved builds an already validated plan in disposable module copies.
// GoArgs must match analysis and omit -o. The caller owns returned Dir cleanup.
// DryRun verifies prepared inputs and plans outputs without backend compilation.
func BuildResolved(ctx context.Context, request ResolvedBuildRequest) (BuildResult, error) {
	var empty BuildResult

	err := validateResolvedBuildRequest(request)
	if err != nil {
		return empty, err
	}

	request.Parent, err = resolvedBuildParent(request.Parent)
	if err != nil {
		return empty, err
	}

	workspaceRequest, err := WorkspaceForAnalysis(request.Code)
	if err != nil {
		return empty, err
	}

	runtime, err := stageResolvedBuildRuntime(request)
	if err != nil {
		return empty, err
	}

	defer func() { _ = os.RemoveAll(runtime.Dir) }()

	workspaceRequest.Runtime, workspaceRequest.Parent = runtime, request.Parent

	prepared, err := PrepareWorkspace(ctx, workspaceRequest)
	if err != nil {
		return empty, err
	}

	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(prepared.Dir)
		}
	}()

	result, err := executeResolvedBuild(ctx, request, workspaceRequest, prepared, runtime)
	if err != nil {
		return empty, err
	}

	complete = true

	return result, nil
}

func validateResolvedBuildRequest(request ResolvedBuildRequest) error {
	if request.DefaultOutput && request.DirectoryOutput {
		return errConflictingResolvedBuildOutputs
	}

	if request.Code == nil {
		return errResolvedBuildWithoutAnalysis
	}

	err := ValidateBuildArguments(request.GoArgs, request.Code.EffectiveBuild)
	if err != nil {
		return err
	}

	for _, argument := range request.GoArgs {
		name, _, _ := strings.Cut(argument, "=")
		if name == "-o" || name == "--o" {
			return errResolvedBuildOutputArgument
		}
	}

	return nil
}

func resolvedBuildParent(parent string) (string, error) {
	if parent == "" {
		return "", nil
	}

	absolute, err := filepath.Abs(parent)
	if err != nil {
		return "", fmt.Errorf("resolve build staging parent: %w", err)
	}

	return absolute, nil
}

func stageResolvedBuildRuntime(request ResolvedBuildRequest) (model.Artifacts, error) {
	var empty model.Artifacts

	files, err := otelc.RenderBundle(request.Backend, request.RuntimeVersion,
		request.Code, request.Plan, "otelplan.local/generated")
	if err != nil {
		return empty, fmt.Errorf("render resolved build runtime: %w", err)
	}

	return StageArtifacts(request.Parent, files)
}

func executeResolvedBuild(ctx context.Context, request ResolvedBuildRequest, workspaceRequest WorkspaceRequest,
	prepared PreparedWorkspace, runtime model.Artifacts) (BuildResult, error) {
	var empty BuildResult

	workingDir := request.WorkingDir
	if workingDir == "" {
		workingDir = request.Code.ModuleRoot
	}

	copiedDir, err := resolvedBuildDirectory(request, prepared, workingDir)
	if err != nil {
		return empty, err
	}

	env, buildFlags, err := RecordedBuildEnvironment(request.Env, request.Code.EffectiveBuild)
	if err != nil {
		return empty, err
	}

	if request.Offline {
		env = append(env, "GOPROXY=off", "GONOPROXY=none", "GOSUMDB=off")
	}

	err = prepareResolvedVendor(ctx, request.Code.EffectiveBuild.ModuleMode, workspaceRequest, prepared, env)
	if err != nil {
		return empty, err
	}

	build, err := resolvedPreparedBuildRequest(ctx, request, prepared, runtime, copiedDir, env)
	if err != nil {
		return empty, err
	}

	output, err := planResolvedBuildOutput(ctx, request, build, workingDir, buildFlags)
	if err != nil {
		return empty, err
	}

	build.GoArgs = output.arguments

	err = BuildPrepared(ctx, build)
	if err != nil {
		return empty, err
	}

	return resolvedBuildResult(prepared.Dir, output, request)
}

func resolvedBuildResult(directory string, output resolvedBuildOutput,
	request ResolvedBuildRequest) (BuildResult, error) {
	if request.DryRun {
		return BuildResult{Dir: directory, Files: nil, PreviewNames: output.previewNames}, nil
	}

	artifacts, err := collectResolvedBuildArtifacts(directory, output, request.DirectoryOutput)
	if err != nil {
		return BuildResult{Dir: "", Files: nil, PreviewNames: nil}, err
	}

	return BuildResult{Dir: directory, Files: artifacts, PreviewNames: nil}, nil
}

func resolvedBuildDirectory(request ResolvedBuildRequest, prepared PreparedWorkspace,
	workingDir string) (string, error) {
	copiedDir, err := prepared.BuildDirectory(workingDir)
	workspaceRoot := request.Code.EffectiveBuild.Workspace &&
		filepath.Clean(workingDir) == filepath.Dir(request.Code.WorkspaceFile)

	if err != nil && workspaceRoot {
		if buildPackageStart(request.GoArgs) >= len(request.GoArgs) {
			return "", errWorkspaceRootWithoutBuildTargets
		}

		return prepared.applicationBuildDirectory()
	}

	return copiedDir, err
}

func prepareResolvedVendor(ctx context.Context, moduleMode string, request WorkspaceRequest,
	prepared PreparedWorkspace, env []string) error {
	if moduleMode != "vendor" {
		return nil
	}

	originalVendor := filepath.Join(request.OriginalWorkspaceDir, "vendor")

	return materializeVendorWorkspace(ctx, prepared, originalVendor, env)
}

func resolvedPreparedBuildRequest(ctx context.Context, request ResolvedBuildRequest,
	prepared PreparedWorkspace, runtime model.Artifacts, copiedDir string, env []string) (PreparedBuildRequest, error) {
	var empty PreparedBuildRequest

	runtimeEnv := append(append([]string(nil), env...), "GOWORK=off")

	runtimeSelection, err := ReadModuleSelection(ctx, runtime.Dir, runtimeEnv)
	if err != nil {
		return empty, fmt.Errorf("read generated runtime module selection: %w", err)
	}

	applicationModules, err := applicationModuleSelection(ctx, prepared, copiedDir, env)
	if err != nil {
		return empty, fmt.Errorf("read isolated application module selection: %w", err)
	}

	err = CheckModuleSelection(request.Code.Modules, applicationModules, prepared.Relocations)
	if err != nil {
		return empty, err
	}

	return PreparedBuildRequest{
		BuildEnvironment: request.Code.EffectiveBuild, Workspace: prepared, ModuleDir: copiedDir,
		Executable: request.Executable, Backend: request.Backend, ApplicationModules: applicationModules,
		RuntimeModules: runtimeSelection, RuntimeOriginalDir: runtime.Dir, Env: env, GoArgs: nil, DryRun: request.DryRun,
	}, nil
}

func planResolvedBuildOutput(ctx context.Context, request ResolvedBuildRequest, build PreparedBuildRequest,
	workingDir string, buildFlags []string) (resolvedBuildOutput, error) {
	var empty resolvedBuildOutput

	args, err := build.Workspace.RelocateBuildArguments(request.GoArgs, workingDir)
	if err != nil {
		return empty, err
	}

	output := resolvedBuildOutput{
		path: filepath.Join(build.Workspace.Dir, "output"), defaultName: "", discard: false,
		arguments: args, previewNames: nil,
	}

	err = output.planNames(ctx, request, build, workingDir, buildFlags)
	if err != nil {
		return empty, err
	}

	if request.DirectoryOutput {
		err := os.Mkdir(output.path, privateBuildOutputDirectoryMode)
		if err != nil {
			return empty, fmt.Errorf("create resolved build output directory: %w", err)
		}
	}

	if !output.discard {
		output.arguments = append([]string{"-o", output.path}, args...)
	}

	return output, nil
}

func (output *resolvedBuildOutput) planNames(ctx context.Context, request ResolvedBuildRequest,
	build PreparedBuildRequest, workingDir string, buildFlags []string) error {
	if !request.DefaultOutput {
		if !request.DryRun {
			return nil
		}

		names, err := resolvedPreviewBuildOutputs(ctx, request, build, workingDir, buildFlags)
		output.previewNames = names

		return err
	}

	name, err := resolvedDefaultBuildOutput(ctx, request, build, workingDir, buildFlags)
	if err != nil {
		return err
	}

	output.defaultName, output.discard = name, name == ""
	if request.DryRun && !output.discard {
		output.previewNames = []string{name}
	}

	return nil
}

func resolvedPreviewBuildOutputs(ctx context.Context, request ResolvedBuildRequest, build PreparedBuildRequest,
	workingDir string, buildFlags []string) ([]string, error) {
	packages := request.GoArgs[buildPackageStart(request.GoArgs):]

	targets, err := build.Workspace.RelocateBuildArguments(packages, workingDir)
	if err != nil {
		return nil, err
	}

	env := append(append([]string(nil), build.Env...), "GOWORK="+build.Workspace.WorkspaceFile)

	data, err := queryBuildTargets(ctx, build.ModuleDir, env, buildFlags, targets,
		request.Code.EffectiveBuild.ModuleMode)
	if err != nil {
		return nil, err
	}

	return decodePreviewBuildOutputs(data, request.Code.EffectiveBuild.GOOS, request.DirectoryOutput)
}

func resolvedDefaultBuildOutput(ctx context.Context, request ResolvedBuildRequest, build PreparedBuildRequest,
	workingDir string, buildFlags []string) (string, error) {
	targets, err := build.Workspace.RelocateBuildArguments(request.Packages, workingDir)
	if err != nil {
		return "", err
	}

	env := append(append([]string(nil), build.Env...), "GOWORK="+build.Workspace.WorkspaceFile)

	return defaultBuildOutput(ctx, build.ModuleDir, env, buildFlags, targets,
		request.Code.EffectiveBuild.GOOS, request.Code.EffectiveBuild.ModuleMode)
}

func collectResolvedBuildArtifacts(directory string, output resolvedBuildOutput,
	directoryOutput bool) ([]BuildArtifact, error) {
	if output.discard {
		return nil, nil
	}

	paths := []string{output.path}

	if directoryOutput {
		var err error

		paths, err = resolvedBuildOutputFiles(output.path)
		if err != nil {
			return nil, err
		}
	}

	var artifacts []BuildArtifact

	for _, filename := range paths {
		name := output.defaultName
		if directoryOutput {
			name = filepath.Base(filename)
		}

		artifact, err := readBuildArtifact(directory, filename, name)
		if err != nil {
			return nil, err
		}

		artifacts = append(artifacts, artifact)
	}

	return artifacts, nil
}

func resolvedBuildOutputFiles(directory string) ([]string, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open resolved build output directory: %w", err)
	}

	defer func() { _ = root.Close() }()

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, fmt.Errorf("read resolved build output directory: %w", err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(directory, entry.Name()))
	}

	return paths, nil
}
