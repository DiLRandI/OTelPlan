package compiler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"
)

// CollectWorkspaceSources finds the module directories needed to relocate a
// workspace, including local replacements. It only reads module manifests.
func CollectWorkspaceSources(ctx context.Context, workspace []byte, workspaceDir string, alternateModFiles map[string]string) ([]string, error) {
	if !filepath.IsAbs(workspaceDir) {
		return nil, fmt.Errorf("workspace directory must be absolute")
	}
	for dir, path := range alternateModFiles {
		if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir || !filepath.IsAbs(path) || !strings.HasSuffix(path, ".mod") {
			return nil, fmt.Errorf("alternate module file requires absolute module and .mod paths")
		}
	}
	work, err := modfile.ParseWork("go.work", workspace, nil)
	if err != nil {
		return nil, fmt.Errorf("parse workspace manifest: %w", err)
	}
	if len(work.Use) == 0 {
		return nil, fmt.Errorf("workspace requires application modules")
	}
	resolve := func(base, path string) string {
		if !filepath.IsAbs(path) {
			path = filepath.Join(base, path)
		}
		return filepath.Clean(path)
	}
	pending := []string{}
	for _, use := range work.Use {
		pending = append(pending, resolve(workspaceDir, use.Path))
	}
	for _, replacement := range work.Replace {
		if replacement.New.Version == "" {
			pending = append(pending, resolve(workspaceDir, replacement.New.Path))
		}
	}
	seen := map[string]bool{}
	physicalDirs := map[string]string{}
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		dir := pending[0]
		pending = pending[1:]
		if seen[dir] {
			continue
		}
		physical, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return nil, fmt.Errorf("resolve source module directory")
		}
		if previous, ok := physicalDirs[physical]; ok && previous != dir {
			return nil, fmt.Errorf("source module is referenced through different filesystem aliases")
		}
		physicalDirs[physical] = dir
		seen[dir] = true
		path := filepath.Join(dir, "go.mod")
		if alternate, ok := alternateModFiles[dir]; ok {
			path = alternate
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read source module manifest")
		}
		module, err := modfile.Parse("go.mod", data, nil)
		if err != nil {
			return nil, fmt.Errorf("parse source module manifest: %w", err)
		}
		if module.Module == nil {
			return nil, fmt.Errorf("source module manifest requires module directive")
		}
		for _, replacement := range module.Replace {
			if replacement.New.Version == "" {
				pending = append(pending, resolve(dir, replacement.New.Path))
			}
		}
	}
	for dir := range alternateModFiles {
		if !seen[dir] {
			return nil, fmt.Errorf("alternate manifest references an unknown source module")
		}
	}
	dirs := make([]string, 0, len(seen))
	for dir := range seen {
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)
	return dirs, nil
}
