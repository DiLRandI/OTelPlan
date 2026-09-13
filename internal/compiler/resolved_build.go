package compiler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
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
}

type BuildArtifact struct {
	Dir    string
	File   string
	Digest string
}

// BuildResolved builds an already validated plan in disposable module copies.
// GoArgs must match analysis and omit -o. The caller owns returned Dir cleanup.
func BuildResolved(ctx context.Context, request ResolvedBuildRequest) (BuildArtifact, error) {
	if request.Code == nil {
		return BuildArtifact{}, fmt.Errorf("build requires analysis")
	}
	if request.Code.EffectiveBuild.ModuleMode == "vendor" {
		return BuildArtifact{}, fmt.Errorf("vendored builds require isolated vendor materialization")
	}
	if err := ValidateBuildArguments(request.GoArgs, request.Code.EffectiveBuild); err != nil {
		return BuildArtifact{}, err
	}
	for _, arg := range request.GoArgs {
		name, _, _ := strings.Cut(arg, "=")
		if name == "-o" || name == "--o" {
			return BuildArtifact{}, fmt.Errorf("resolved build owns its temporary output path")
		}
	}
	if request.Parent != "" {
		parent, err := filepath.Abs(request.Parent)
		if err != nil {
			return BuildArtifact{}, err
		}
		request.Parent = parent
	}
	workspaceRequest, err := WorkspaceForAnalysis(request.Code)
	if err != nil {
		return BuildArtifact{}, err
	}
	files, err := otelc.RenderBundle(request.Backend, request.RuntimeVersion, request.Code, request.Plan, "otelplan.local/generated")
	if err != nil {
		return BuildArtifact{}, err
	}
	runtime, err := StageArtifacts(request.Parent, files)
	if err != nil {
		return BuildArtifact{}, err
	}
	defer func() { _ = os.RemoveAll(runtime.Dir) }()
	workspaceRequest.Runtime, workspaceRequest.Parent = runtime, request.Parent
	prepared, err := PrepareWorkspace(ctx, workspaceRequest)
	if err != nil {
		return BuildArtifact{}, err
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
	if err != nil {
		return BuildArtifact{}, err
	}
	env, _, err := RecordedBuildEnvironment(request.Env, request.Code.EffectiveBuild)
	if err != nil {
		return BuildArtifact{}, err
	}
	if request.Offline {
		env = append(env, "GOPROXY=off", "GONOPROXY=none", "GOSUMDB=off")
	}
	runtimeSelection, err := ReadModuleSelection(ctx, runtime.Dir, append(append([]string(nil), env...), "GOWORK=off"))
	if err != nil {
		return BuildArtifact{}, err
	}
	output := filepath.Join(prepared.Dir, "output")
	args := append([]string{"-o", output}, request.GoArgs...)
	err = BuildPrepared(ctx, PreparedBuildRequest{BuildEnvironment: request.Code.EffectiveBuild, Workspace: prepared, ModuleDir: copiedDir, Executable: request.Executable, Backend: request.Backend, ApplicationModules: request.Code.Modules, RuntimeModules: runtimeSelection, RuntimeOriginalDir: runtime.Dir, Env: env, GoArgs: args})
	if err != nil {
		return BuildArtifact{}, err
	}
	file, err := os.Open(output)
	if err != nil {
		return BuildArtifact{}, fmt.Errorf("backend did not produce build output")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return BuildArtifact{}, fmt.Errorf("backend output is not a regular file")
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return BuildArtifact{}, err
	}
	complete = true
	return BuildArtifact{Dir: prepared.Dir, File: output, Digest: fmt.Sprintf("sha256:%x", digest.Sum(nil))}, nil
}
