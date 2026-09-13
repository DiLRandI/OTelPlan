package compiler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

type WorkspaceRequest struct {
	SourceDirs []string
	// AlternateModFiles maps original module directories to absolute .mod files.
	AlternateModFiles    map[string]string
	OriginalWorkspaceDir string
	Workspace            []byte
	WorkspaceSums        []byte
	Runtime              model.Artifacts
	Parent               string
}

type PreparedWorkspace struct {
	Dir           string
	WorkspaceFile string
	Relocations   map[string]string
	Runtime       model.Artifacts
}

// PrepareWorkspace creates disposable module and runtime copies. The caller
// supplies every workspace/local-replacement module and owns result.Dir cleanup.
func PrepareWorkspace(ctx context.Context, request WorkspaceRequest) (PreparedWorkspace, error) {
	if err := VerifyArtifacts(request.Runtime); err != nil {
		return PreparedWorkspace{}, err
	}
	dirs := append([]string(nil), request.SourceDirs...)
	sort.Strings(dirs)
	for i, dir := range dirs {
		if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir || (i > 0 && dirs[i-1] == dir) {
			return PreparedWorkspace{}, fmt.Errorf("source module directories must be unique absolute paths")
		}
	}
	if len(dirs) == 0 {
		return PreparedWorkspace{}, fmt.Errorf("build workspace requires application modules")
	}
	for dir, path := range request.AlternateModFiles {
		if !slices.Contains(dirs, dir) || !filepath.IsAbs(path) || !strings.HasSuffix(path, ".mod") {
			return PreparedWorkspace{}, fmt.Errorf("alternate module file requires a known module and an absolute .mod path")
		}
	}
	stagingParent := request.Parent
	if stagingParent == "" {
		stagingParent = os.TempDir()
	}
	stagingParent, err := filepath.Abs(stagingParent)
	if err != nil {
		return PreparedWorkspace{}, fmt.Errorf("resolve build staging parent: %w", err)
	}
	parent, err := os.MkdirTemp(stagingParent, "otelplan-build-")
	if err != nil {
		return PreparedWorkspace{}, fmt.Errorf("create build workspace: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(parent)
		}
	}()
	result := PreparedWorkspace{Dir: parent, WorkspaceFile: filepath.Join(parent, "go.work"), Relocations: map[string]string{}}
	for _, dir := range dirs {
		copied, err := CopySourceTree(ctx, dir, parent)
		if err != nil {
			return PreparedWorkspace{}, err
		}
		result.Relocations[dir] = copied
	}
	for _, dir := range dirs {
		path := filepath.Join(result.Relocations[dir], "go.mod")
		if alternate, ok := request.AlternateModFiles[dir]; ok {
			if err := installAlternateModuleFiles(alternate, result.Relocations[dir]); err != nil {
				return PreparedWorkspace{}, err
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return PreparedWorkspace{}, fmt.Errorf("read copied module manifest: %w", err)
		}
		relocated, err := RelocateModuleManifest(data, dir, result.Relocations)
		if err != nil {
			return PreparedWorkspace{}, err
		}
		if err := os.WriteFile(path, relocated, 0600); err != nil {
			return PreparedWorkspace{}, fmt.Errorf("write copied module manifest: %w", err)
		}
	}
	runtimeDir, err := CopySourceTree(ctx, request.Runtime.Dir, parent)
	if err != nil {
		return PreparedWorkspace{}, err
	}
	result.Runtime = model.Artifacts{Dir: runtimeDir, Files: append([]model.ArtifactFile(nil), request.Runtime.Files...)}
	if err := VerifyArtifacts(result.Runtime); err != nil {
		return PreparedWorkspace{}, err
	}
	workspace, err := RelocateWorkspaceManifest(request.Workspace, request.OriginalWorkspaceDir, runtimeDir, result.Relocations)
	if err != nil {
		return PreparedWorkspace{}, err
	}
	if err := os.WriteFile(result.WorkspaceFile, workspace, 0600); err != nil {
		return PreparedWorkspace{}, fmt.Errorf("write isolated workspace: %w", err)
	}
	if len(request.WorkspaceSums) > 0 {
		if err := os.WriteFile(result.WorkspaceFile+".sum", request.WorkspaceSums, 0600); err != nil {
			return PreparedWorkspace{}, fmt.Errorf("write isolated workspace sums: %w", err)
		}
	}
	complete = true
	return result, nil
}
