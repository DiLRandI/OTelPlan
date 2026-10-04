package compiler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"
)

var (
	errRelativeWorkspaceSourceDir    = errors.New("workspace directory must be absolute")
	errInvalidAlternateSourcePath    = errors.New("alternate module file requires absolute module and .mod paths")
	errNoWorkspaceApplicationModules = errors.New("workspace requires application modules")
	errAliasedSourceModule           = errors.New("source module is referenced through different filesystem aliases")
	errMissingSourceModuleDirective  = errors.New("source module manifest requires module directive")
	errUnknownAlternateSourceModule  = errors.New("alternate manifest references an unknown source module")
)

// CollectWorkspaceSources finds the module directories needed to relocate a
// workspace, including local replacements. It only reads module manifests.
func CollectWorkspaceSources(ctx context.Context, workspace []byte, workspaceDir string,
	alternateModFiles map[string]string) ([]string, error) {
	err := validateWorkspaceSourcePaths(workspaceDir, alternateModFiles)
	if err != nil {
		return nil, err
	}

	work, err := modfile.ParseWork("go.work", workspace, nil)
	if err != nil {
		return nil, fmt.Errorf("parse workspace manifest: %w", err)
	}

	if len(work.Use) == 0 {
		return nil, errNoWorkspaceApplicationModules
	}

	pending := workspaceModuleQueue(work, workspaceDir)

	seen, err := collectWorkspaceModuleDirs(ctx, pending, alternateModFiles)
	if err != nil {
		return nil, err
	}

	for dir := range alternateModFiles {
		if !seen[dir] {
			return nil, errUnknownAlternateSourceModule
		}
	}

	dirs := make([]string, 0, len(seen))
	for dir := range seen {
		dirs = append(dirs, dir)
	}

	slices.Sort(dirs)

	return dirs, nil
}

func validateWorkspaceSourcePaths(workspaceDir string, alternateModFiles map[string]string) error {
	if !filepath.IsAbs(workspaceDir) {
		return errRelativeWorkspaceSourceDir
	}

	for dir, path := range alternateModFiles {
		if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir || !filepath.IsAbs(path) || !strings.HasSuffix(path, ".mod") {
			return errInvalidAlternateSourcePath
		}
	}

	return nil
}

func resolveWorkspaceModulePath(base, path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}

	return filepath.Clean(path)
}

func workspaceModuleQueue(work *modfile.WorkFile, directory string) []string {
	pending := make([]string, 0, len(work.Use)+len(work.Replace))
	for _, use := range work.Use {
		pending = append(pending, resolveWorkspaceModulePath(directory, use.Path))
	}

	return append(pending, localReplacementDirs(work.Replace, directory)...)
}

func localReplacementDirs(replacements []*modfile.Replace, directory string) []string {
	var dirs []string

	for _, replacement := range replacements {
		if replacement.New.Version == "" {
			dirs = append(dirs, resolveWorkspaceModulePath(directory, replacement.New.Path))
		}
	}

	return dirs
}

func collectWorkspaceModuleDirs(ctx context.Context, pending []string,
	alternateModFiles map[string]string) (map[string]bool, error) {
	seen := map[string]bool{}
	physicalDirs := map[string]string{}

	for len(pending) > 0 {
		err := ctx.Err()
		if err != nil {
			return nil, fmt.Errorf("collect workspace source modules: %w", err)
		}

		dir := pending[0]
		pending = pending[1:]

		if seen[dir] {
			continue
		}

		physical, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return nil, fmt.Errorf("resolve source module directory: %w", err)
		}

		if previous, exists := physicalDirs[physical]; exists && previous != dir {
			return nil, errAliasedSourceModule
		}

		physicalDirs[physical], seen[dir] = dir, true

		module, err := readWorkspaceSourceModule(dir, alternateModFiles)
		if err != nil {
			return nil, err
		}

		pending = append(pending, localReplacementDirs(module.Replace, dir)...)
	}

	return seen, nil
}

func readWorkspaceSourceModule(directory string, alternateModFiles map[string]string) (*modfile.File, error) {
	path := filepath.Join(directory, "go.mod")
	if alternate, exists := alternateModFiles[directory]; exists {
		path = alternate
	}

	data, err := readWorkspaceSourceManifest(path)
	if err != nil {
		return nil, fmt.Errorf("read source module manifest: %w", err)
	}

	module, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, fmt.Errorf("parse source module manifest: %w", err)
	}

	if module.Module == nil {
		return nil, errMissingSourceModuleDirective
	}

	return module, nil
}

func readWorkspaceSourceManifest(path string) ([]byte, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("resolve source manifest input: %w", err)
	}

	root, err := os.OpenRoot(filepath.Dir(resolved))
	if err != nil {
		return nil, fmt.Errorf("open source manifest input directory: %w", err)
	}

	defer func() { _ = root.Close() }()

	data, err := root.ReadFile(filepath.Base(resolved))
	if err != nil {
		return nil, fmt.Errorf("read source manifest input: %w", err)
	}

	return data, nil
}
