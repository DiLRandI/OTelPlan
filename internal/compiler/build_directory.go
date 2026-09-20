package compiler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// BuildDirectory maps an original working directory into its prepared module.
func (workspace PreparedWorkspace) BuildDirectory(original string) (string, error) {
	if !filepath.IsAbs(original) {
		return "", errors.New("original build directory must be absolute")
	}

	original = filepath.Clean(original)

	source, copied := "", ""

	for dir, replacement := range workspace.Relocations {
		if filepath.IsAbs(dir) && withinTree(dir, original) && len(dir) > len(source) {
			source, copied = dir, replacement
		}
	}

	if source == "" {
		return "", errors.New("build directory has no prepared module")
	}

	relative, err := filepath.Rel(source, original)
	if err != nil {
		return "", err
	}

	result := filepath.Join(copied, relative)
	if err := workspace.validateBuildDirectory(result); err != nil {
		return "", err
	}

	return result, nil
}

func (workspace PreparedWorkspace) validateBuildDirectory(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("build directory must be absolute")
	}

	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return errors.New("resolve prepared build directory")
	}

	info, err := os.Stat(physical)
	if err != nil || !info.IsDir() {
		return errors.New("prepared build directory does not exist")
	}

	workspaceRoot, err := filepath.EvalSymlinks(workspace.Dir)
	if err != nil {
		return errors.New("resolve prepared workspace directory")
	}

	for _, dir := range workspace.Relocations {
		if !filepath.IsAbs(dir) || !withinTree(workspace.Dir, dir) || !withinTree(dir, path) {
			continue
		}

		root, err := filepath.EvalSymlinks(dir)
		if err == nil && withinTree(workspaceRoot, root) && withinTree(root, physical) {
			return nil
		}
	}

	return errors.New("build directory is not inside a prepared application module")
}

func (workspace PreparedWorkspace) applicationBuildDirectory() (string, error) {
	data, err := os.ReadFile(workspace.WorkspaceFile)
	if err != nil {
		return "", fmt.Errorf("read prepared workspace: %w", err)
	}

	work, err := modfile.ParseWork(workspace.WorkspaceFile, data, nil)
	if err != nil {
		return "", errors.New("parse prepared workspace")
	}

	for _, use := range work.Use {
		dir := use.Path
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(filepath.Dir(workspace.WorkspaceFile), dir)
		}

		if filepath.Clean(dir) == filepath.Clean(workspace.Runtime.Dir) {
			continue
		}

		err := workspace.validateBuildDirectory(dir)

		if err != nil {
			return "", err
		}

		return dir, nil
	}

	return "", errors.New("workspace has no prepared application module")
}
