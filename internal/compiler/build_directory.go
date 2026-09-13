package compiler

import (
	"fmt"
	"os"
	"path/filepath"
)

// BuildDirectory maps an original working directory into its prepared module.
func (workspace PreparedWorkspace) BuildDirectory(original string) (string, error) {
	if !filepath.IsAbs(original) {
		return "", fmt.Errorf("original build directory must be absolute")
	}
	original = filepath.Clean(original)
	source, copied := "", ""
	for dir, replacement := range workspace.Relocations {
		if filepath.IsAbs(dir) && withinTree(dir, original) && len(dir) > len(source) {
			source, copied = dir, replacement
		}
	}
	if source == "" {
		return "", fmt.Errorf("build directory has no prepared module")
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
		return fmt.Errorf("build directory must be absolute")
	}
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve prepared build directory")
	}
	info, err := os.Stat(physical)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("prepared build directory does not exist")
	}
	workspaceRoot, err := filepath.EvalSymlinks(workspace.Dir)
	if err != nil {
		return fmt.Errorf("resolve prepared workspace directory")
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
	return fmt.Errorf("build directory is not inside a prepared application module")
}
