package compiler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

var (
	errUnpreparedBuildModule          = errors.New("build directory has no prepared module")
	errRelativePreparedBuildDirectory = errors.New("build directory must be absolute")
	errMissingPreparedBuildDirectory  = errors.New("prepared build directory does not exist")
	errOutsidePreparedApplication     = errors.New("build directory is not inside a prepared application module")
	errMissingPreparedApplication     = errors.New("workspace has no prepared application module")
)

// BuildDirectory maps an original working directory into its prepared module.
func (workspace PreparedWorkspace) BuildDirectory(original string) (string, error) {
	if !filepath.IsAbs(original) {
		return "", errRelativeOriginalBuildDirectory
	}

	original = filepath.Clean(original)

	source, copied := "", ""

	for dir, replacement := range workspace.Relocations {
		if filepath.IsAbs(dir) && withinTree(dir, original) && len(dir) > len(source) {
			source, copied = dir, replacement
		}
	}

	if source == "" {
		return "", errUnpreparedBuildModule
	}

	relative, err := filepath.Rel(source, original)
	if err != nil {
		return "", fmt.Errorf("map original build directory: %w", err)
	}

	result := filepath.Join(copied, relative)

	err = workspace.validateBuildDirectory(result)
	if err != nil {
		return "", err
	}

	return result, nil
}

func (workspace PreparedWorkspace) validateBuildDirectory(path string) error {
	if !filepath.IsAbs(path) {
		return errRelativePreparedBuildDirectory
	}

	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve prepared build directory: %w", err)
	}

	info, err := statPreparedDirectory(physical)
	if err != nil {
		return fmt.Errorf("inspect prepared build directory: %w", err)
	}

	if !info.IsDir() {
		return errMissingPreparedBuildDirectory
	}

	workspaceRoot, err := filepath.EvalSymlinks(workspace.Dir)
	if err != nil {
		return fmt.Errorf("resolve prepared workspace directory: %w", err)
	}

	if workspace.containsPreparedBuildDirectory(path, physical, workspaceRoot) {
		return nil
	}

	return errOutsidePreparedApplication
}

func statPreparedDirectory(path string) (os.FileInfo, error) {
	parent, name := filepath.Dir(path), filepath.Base(path)
	if parent == path {
		name = "."
	}

	root, err := os.OpenRoot(parent)
	if err != nil {
		return nil, fmt.Errorf("open prepared directory parent: %w", err)
	}

	defer func() { _ = root.Close() }()

	info, err := root.Stat(name)
	if err != nil {
		return nil, fmt.Errorf("stat prepared directory: %w", err)
	}

	return info, nil
}

func (workspace PreparedWorkspace) containsPreparedBuildDirectory(path, physical, workspaceRoot string) bool {
	for _, dir := range workspace.Relocations {
		if !filepath.IsAbs(dir) || !withinTree(workspace.Dir, dir) || !withinTree(dir, path) {
			continue
		}

		root, err := filepath.EvalSymlinks(dir)
		if err == nil && withinTree(workspaceRoot, root) && withinTree(root, physical) {
			return true
		}
	}

	return false
}

func (workspace PreparedWorkspace) applicationBuildDirectory() (string, error) {
	data, err := os.ReadFile(workspace.WorkspaceFile)
	if err != nil {
		return "", fmt.Errorf("read prepared workspace: %w", err)
	}

	work, err := modfile.ParseWork(workspace.WorkspaceFile, data, nil)
	if err != nil {
		return "", fmt.Errorf("parse prepared workspace: %w", err)
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

	return "", errMissingPreparedApplication
}
