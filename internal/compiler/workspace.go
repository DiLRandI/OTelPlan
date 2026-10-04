package compiler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const isolatedWorkspaceFileMode = 0o600

var (
	errInvalidWorkspaceModuleDirs  = errors.New("source module directories must be unique absolute paths")
	errWorkspaceWithoutApplication = errors.New("build workspace requires application modules")
	errInvalidWorkspaceAlternate   = errors.New("alternate module file requires a known module and an absolute .mod path")
)

// WorkspaceRequest describes source modules and verified runtime artifacts to copy.
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

// PreparedWorkspace owns disposable copies and maps source modules to their copies.
type PreparedWorkspace struct {
	Dir           string
	WorkspaceFile string
	Relocations   map[string]string
	Runtime       model.Artifacts
}

// PrepareWorkspace creates disposable module and runtime copies. The caller
// may supply an explicit source directory list and owns result.Dir cleanup.
// Otherwise the directories are collected from workspace and module manifests.
func PrepareWorkspace(ctx context.Context, request WorkspaceRequest) (PreparedWorkspace, error) {
	var empty PreparedWorkspace

	err := VerifyArtifacts(request.Runtime)
	if err != nil {
		return empty, err
	}

	dirs, err := workspaceSourceDirectories(ctx, request)
	if err != nil {
		return empty, err
	}

	parent, err := createBuildWorkspaceDirectory(request.Parent)
	if err != nil {
		return empty, err
	}

	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(parent)
		}
	}()

	relocations, err := prepareWorkspaceSourceCopies(ctx, request, dirs, parent)
	if err != nil {
		return empty, err
	}

	runtime, err := copyVerifiedWorkspaceRuntime(ctx, request.Runtime, parent)
	if err != nil {
		return empty, err
	}

	err = writeIsolatedWorkspace(request, runtime.Dir, relocations, parent)
	if err != nil {
		return empty, err
	}

	complete = true

	return PreparedWorkspace{
		Dir: parent, WorkspaceFile: filepath.Join(parent, "go.work"), Relocations: relocations, Runtime: runtime,
	}, nil
}

func workspaceSourceDirectories(ctx context.Context, request WorkspaceRequest) ([]string, error) {
	dirs := slices.Clone(request.SourceDirs)
	if len(dirs) == 0 {
		var err error

		dirs, err = CollectWorkspaceSources(ctx, request.Workspace, request.OriginalWorkspaceDir, request.AlternateModFiles)
		if err != nil {
			return nil, err
		}
	}

	slices.Sort(dirs)

	err := validateWorkspaceSourceDirectories(dirs)
	if err != nil {
		return nil, err
	}

	err = validateWorkspaceAlternateFiles(dirs, request.AlternateModFiles)
	if err != nil {
		return nil, err
	}

	return dirs, nil
}

func validateWorkspaceSourceDirectories(dirs []string) error {
	for index, dir := range dirs {
		if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir || (index > 0 && dirs[index-1] == dir) {
			return errInvalidWorkspaceModuleDirs
		}
	}

	if len(dirs) == 0 {
		return errWorkspaceWithoutApplication
	}

	return nil
}

func validateWorkspaceAlternateFiles(dirs []string, alternateModFiles map[string]string) error {
	for dir, path := range alternateModFiles {
		if !slices.Contains(dirs, dir) || !filepath.IsAbs(path) || !strings.HasSuffix(path, ".mod") {
			return errInvalidWorkspaceAlternate
		}
	}

	return nil
}

func createBuildWorkspaceDirectory(parent string) (string, error) {
	if parent == "" {
		parent = os.TempDir()
	}

	parent, err := filepath.Abs(parent)
	if err != nil {
		return "", fmt.Errorf("resolve build staging parent: %w", err)
	}

	dir, err := os.MkdirTemp(parent, "otelplan-build-")
	if err != nil {
		return "", fmt.Errorf("create build workspace: %w", err)
	}

	return dir, nil
}

func prepareWorkspaceSourceCopies(ctx context.Context, request WorkspaceRequest,
	dirs []string, parent string) (map[string]string, error) {
	relocations := make(map[string]string, len(dirs))
	for _, dir := range dirs {
		copied, err := CopySourceTree(ctx, dir, parent)
		if err != nil {
			return nil, err
		}

		relocations[dir] = copied
	}

	for _, dir := range dirs {
		alternate := request.AlternateModFiles[dir]

		err := relocateCopiedWorkspaceModule(dir, relocations[dir], alternate, relocations)
		if err != nil {
			return nil, err
		}
	}

	return relocations, nil
}

func relocateCopiedWorkspaceModule(originalDir, copiedDir, alternate string, relocations map[string]string) error {
	if alternate != "" {
		err := installAlternateModuleFiles(alternate, copiedDir)
		if err != nil {
			return err
		}
	}

	root, err := os.OpenRoot(copiedDir)
	if err != nil {
		return fmt.Errorf("open copied module directory: %w", err)
	}

	defer func() { _ = root.Close() }()

	data, err := root.ReadFile("go.mod")
	if err != nil {
		return fmt.Errorf("read copied module manifest: %w", err)
	}

	relocated, err := RelocateModuleManifest(data, originalDir, relocations)
	if err != nil {
		return err
	}

	err = root.WriteFile("go.mod", relocated, isolatedWorkspaceFileMode)
	if err != nil {
		return fmt.Errorf("write copied module manifest: %w", err)
	}

	return nil
}

func copyVerifiedWorkspaceRuntime(ctx context.Context, original model.Artifacts,
	parent string) (model.Artifacts, error) {
	var empty model.Artifacts

	dir, err := CopySourceTree(ctx, original.Dir, parent)
	if err != nil {
		return empty, err
	}

	copied := model.Artifacts{Dir: dir, Files: slices.Clone(original.Files)}

	err = VerifyArtifacts(copied)
	if err != nil {
		return empty, err
	}

	return copied, nil
}

func writeIsolatedWorkspace(request WorkspaceRequest, runtimeDir string,
	relocations map[string]string, parent string) error {
	workspace, err := RelocateWorkspaceManifest(request.Workspace, request.OriginalWorkspaceDir, runtimeDir, relocations)
	if err != nil {
		return err
	}

	root, err := os.OpenRoot(parent)
	if err != nil {
		return fmt.Errorf("open isolated workspace directory: %w", err)
	}

	defer func() { _ = root.Close() }()

	err = root.WriteFile("go.work", workspace, isolatedWorkspaceFileMode)
	if err != nil {
		return fmt.Errorf("write isolated workspace: %w", err)
	}

	if len(request.WorkspaceSums) > 0 {
		err := root.WriteFile("go.work.sum", request.WorkspaceSums, isolatedWorkspaceFileMode)
		if err != nil {
			return fmt.Errorf("write isolated workspace sums: %w", err)
		}
	}

	return nil
}
