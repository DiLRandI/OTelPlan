package compiler

import (
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

var (
	errRelativeWorkspaceDirectories    = errors.New("workspace directories must be absolute")
	errUncopiedWorkspacePath           = errors.New("workspace path has no isolated copy")
	errDuplicateCopiedWorkspaceModules = errors.New("workspace has duplicate copied modules")
	errOverlappingWorkspaceRuntime     = errors.New("runtime module overlaps application workspace")
)

// RelocateWorkspaceManifest preserves workspace directives while relocating
// every use and local replacement into verified copies. RuntimeDir is the
// separately staged generated module, added as a workspace member.
func RelocateWorkspaceManifest(data []byte, originalDir, runtimeDir string,
	relocations map[string]string) ([]byte, error) {
	if !filepath.IsAbs(originalDir) || !filepath.IsAbs(runtimeDir) {
		return nil, errRelativeWorkspaceDirectories
	}

	work, err := modfile.ParseWork("go.work", data, nil)
	if err != nil {
		return nil, fmt.Errorf("parse workspace manifest: %w", err)
	}

	uses, err := relocatedWorkspaceUses(work.Use, originalDir, runtimeDir, relocations)
	if err != nil {
		return nil, err
	}

	work.SetUse(uses)

	err = relocateWorkspaceReplacements(work, originalDir, relocations)
	if err != nil {
		return nil, err
	}

	work.Cleanup()

	return modfile.Format(work.Syntax), nil
}

func copiedWorkspacePath(path, originalDir string, relocations map[string]string) (string, error) {
	original := resolveWorkspaceModulePath(originalDir, path)

	result, exists := relocations[original]
	if !exists || !filepath.IsAbs(result) {
		return "", errUncopiedWorkspacePath
	}

	return filepath.Clean(result), nil
}

func relocatedWorkspaceUses(original []*modfile.Use, originalDir, runtimeDir string,
	relocations map[string]string) ([]*modfile.Use, error) {
	uses := make([]*modfile.Use, 0, len(original)+1)
	seen := make(map[string]bool, len(original))

	for _, use := range original {
		path, err := copiedWorkspacePath(use.Path, originalDir, relocations)
		if err != nil {
			return nil, err
		}

		if seen[path] {
			return nil, errDuplicateCopiedWorkspaceModules
		}

		seen[path] = true
		uses = append(uses, &modfile.Use{Path: path, ModulePath: use.ModulePath, Syntax: nil})
	}

	runtimeDir = filepath.Clean(runtimeDir)
	if seen[runtimeDir] {
		return nil, errOverlappingWorkspaceRuntime
	}

	uses = append(uses, &modfile.Use{Path: runtimeDir, ModulePath: "", Syntax: nil})

	return uses, nil
}

func relocateWorkspaceReplacements(work *modfile.WorkFile, originalDir string, relocations map[string]string) error {
	for _, replacement := range work.Replace {
		if replacement.New.Version != "" {
			continue
		}

		path, err := copiedWorkspacePath(replacement.New.Path, originalDir, relocations)
		if err != nil {
			return err
		}

		err = work.AddReplace(replacement.Old.Path, replacement.Old.Version, path, "")
		if err != nil {
			return fmt.Errorf("relocate workspace replacement: %w", err)
		}
	}

	return nil
}
