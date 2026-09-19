package compiler

import (
	"fmt"
	"path/filepath"
	"strings"
)

// RelocateBuildArguments maps filesystem package patterns and Go filenames to
// prepared copies. Arguments must first pass ValidateBuildArguments.
func (workspace PreparedWorkspace) RelocateBuildArguments(args []string, originalDir string) ([]string, error) {
	if !filepath.IsAbs(originalDir) {
		return nil, fmt.Errorf("original build directory must be absolute")
	}
	result := append([]string(nil), args...)
	first := buildPackageStart(args)
	for i := first; i < len(result); i++ {
		arg := result[i]
		local := filepath.IsAbs(arg) || arg == "." || arg == ".." || strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") || strings.HasPrefix(arg, ".\\") || strings.HasPrefix(arg, "..\\")
		file := strings.HasSuffix(arg, ".go")
		if !local && !file {
			continue
		}
		path := filepath.Clean(arg)
		directory, suffix := path, ""
		if file {
			directory, suffix = filepath.Dir(path), filepath.Base(path)
		} else if wildcard := strings.Index(path, "..."); wildcard >= 0 {
			separator := strings.LastIndexAny(path[:wildcard], "/\\")
			directory, suffix = path[:separator+1], path[separator+1:]
			if directory == "" {
				directory = "."
			}
		}
		if !filepath.IsAbs(directory) {
			directory = filepath.Join(originalDir, directory)
		}
		copied, err := workspace.BuildDirectory(directory)
		if err != nil {
			return nil, err
		}
		result[i] = filepath.Join(copied, suffix)
	}
	return result, nil
}

func buildPackageStart(args []string) int {
	first := 0
	for first < len(args) && strings.HasPrefix(args[first], "-") {
		name, _, value := strings.Cut(args[first], "=")
		name = strings.TrimPrefix(name, "-")
		name = strings.TrimPrefix(name, "-")
		if !value && (name == "p" || name == "tags" || name == "mod" || name == "o") {
			first++
		}
		first++
	}
	return first
}
