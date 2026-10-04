package compiler

import (
	"errors"
	"path/filepath"
	"strings"
)

var errRelativeOriginalBuildDirectory = errors.New("original build directory must be absolute")

// RelocateBuildArguments maps filesystem package patterns and Go filenames to
// prepared copies. Arguments must first pass ValidateBuildArguments.
func (workspace PreparedWorkspace) RelocateBuildArguments(args []string, originalDir string) ([]string, error) {
	if !filepath.IsAbs(originalDir) {
		return nil, errRelativeOriginalBuildDirectory
	}

	result := append([]string(nil), args...)

	first := buildPackageStart(args)

	for position := first; position < len(result); position++ {
		arg := result[position]
		if !isFilesystemBuildTarget(arg) {
			continue
		}

		relocated, err := workspace.relocateBuildTarget(arg, originalDir)
		if err != nil {
			return nil, err
		}

		result[position] = relocated
	}

	return result, nil
}

func isFilesystemBuildTarget(argument string) bool {
	return filepath.IsAbs(argument) || argument == "." || argument == ".." ||
		strings.HasPrefix(argument, "./") || strings.HasPrefix(argument, "../") ||
		strings.HasPrefix(argument, ".\\") || strings.HasPrefix(argument, "..\\") ||
		strings.HasSuffix(argument, ".go")
}

func (workspace PreparedWorkspace) relocateBuildTarget(argument, originalDir string) (string, error) {
	directory, suffix := buildTargetParts(argument)
	if !filepath.IsAbs(directory) {
		directory = filepath.Join(originalDir, directory)
	}

	copied, err := workspace.BuildDirectory(directory)
	if err != nil {
		return "", err
	}

	return filepath.Join(copied, suffix), nil
}

func buildTargetParts(argument string) (string, string) {
	path := filepath.Clean(argument)

	if strings.HasSuffix(argument, ".go") {
		return filepath.Dir(path), filepath.Base(path)
	}

	before, _, wildcard := strings.Cut(path, "...")
	if !wildcard {
		return path, ""
	}

	separator := strings.LastIndexAny(before, "/\\")

	directory, suffix := path[:separator+1], path[separator+1:]
	if directory == "" {
		directory = "."
	}

	return directory, suffix
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
