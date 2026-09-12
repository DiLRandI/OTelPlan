package compiler

import (
	"fmt"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// RelocateWorkspaceManifest preserves workspace directives while relocating
// every use and local replacement into verified copies. RuntimeDir is the
// separately staged generated module, added as a workspace member.
func RelocateWorkspaceManifest(data []byte, originalDir, runtimeDir string, relocations map[string]string) ([]byte, error) {
	if !filepath.IsAbs(originalDir) || !filepath.IsAbs(runtimeDir) {
		return nil, fmt.Errorf("workspace directories must be absolute")
	}
	work, err := modfile.ParseWork("go.work", data, nil)
	if err != nil {
		return nil, fmt.Errorf("parse workspace manifest: %w", err)
	}
	relocated := func(path string) (string, error) {
		if !filepath.IsAbs(path) {
			path = filepath.Join(originalDir, path)
		}
		result, ok := relocations[filepath.Clean(path)]
		if !ok || !filepath.IsAbs(result) {
			return "", fmt.Errorf("workspace path has no isolated copy")
		}
		return filepath.Clean(result), nil
	}
	uses := make([]*modfile.Use, 0, len(work.Use)+1)
	seen := map[string]bool{}
	for _, use := range work.Use {
		path, err := relocated(use.Path)
		if err != nil {
			return nil, err
		}
		if seen[path] {
			return nil, fmt.Errorf("workspace has duplicate copied modules")
		}
		seen[path] = true
		uses = append(uses, &modfile.Use{Path: path, ModulePath: use.ModulePath})
	}
	runtimeDir = filepath.Clean(runtimeDir)
	if seen[runtimeDir] {
		return nil, fmt.Errorf("runtime module overlaps application workspace")
	}
	uses = append(uses, &modfile.Use{Path: runtimeDir})
	work.SetUse(uses)
	for _, replacement := range work.Replace {
		if replacement.New.Version != "" {
			continue
		}
		path, err := relocated(replacement.New.Path)
		if err != nil {
			return nil, err
		}
		if err := work.AddReplace(replacement.Old.Path, replacement.Old.Version, path, ""); err != nil {
			return nil, fmt.Errorf("relocate workspace replacement: %w", err)
		}
	}
	work.Cleanup()
	return modfile.Format(work.Syntax), nil
}
