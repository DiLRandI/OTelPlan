package compiler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/module"
)

const targetQueryVendorMode = "vendor"

var (
	errCommandWithoutSources        = errors.New("command has no source files")
	errUnsupportedTargetQueryFlag   = errors.New("unsupported target query flag")
	errTargetQueryFlagValueRequired = errors.New("target query flag requires a value")
	errInvalidTargetQueryModuleMode = errors.New("unsupported target query module mode")
	errSingleFileBuildTargets       = errors.New("single-file build output requires exactly one package")
	errDirectoryBuildWithoutMain    = errors.New("directory build output requires a main package")
)

type buildTargetInfo struct {
	Name       string   `json:"Name"`
	ImportPath string   `json:"ImportPath"`
	GoFiles    []string `json:"GoFiles"`
	CgoFiles   []string `json:"CgoFiles"`
}

func defaultBuildOutput(ctx context.Context, dir string, env, flags, targets []string,
	goos, moduleMode string) (string, error) {
	data, err := queryBuildTargets(ctx, dir, env, flags, targets, moduleMode)
	if err != nil {
		return "", err
	}

	return decodeDefaultBuildOutput(data, goos)
}

func queryBuildTargets(ctx context.Context, dir string, env, flags, targets []string,
	moduleMode string) ([]byte, error) {
	err := validateTargetQueryFlags(flags, moduleMode)
	if err != nil {
		return nil, err
	}

	command := exec.CommandContext(ctx, "go", "list", "-json")
	command.Args = append(command.Args, "-mod="+moduleMode)
	command.Args = append(command.Args, flags...)
	command.Args = append(command.Args, "--")
	command.Args = append(command.Args, targets...)
	command.Dir, command.Env = dir, env

	data, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("read build targets: %w", err)
	}

	return data, nil
}

func validateTargetQueryFlags(flags []string, moduleMode string) error {
	switch moduleMode {
	case "readonly", "mod", targetQueryVendorMode:
	default:
		return errInvalidTargetQueryModuleMode
	}

	for position := 0; position < len(flags); position++ {
		if !strings.HasPrefix(flags[position], "-") {
			return errUnsupportedTargetQueryFlag
		}

		name, value, hasValue := strings.Cut(flags[position], "=")
		name = strings.TrimPrefix(name, "-")

		name = "-" + strings.TrimPrefix(name, "-")
		if name == buildTagsFlag && !hasValue {
			position++
			if position >= len(flags) {
				return errTargetQueryFlagValueRequired
			}

			continue
		}

		err := validateTargetQueryFlag(name, value, hasValue)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateTargetQueryFlag(name, value string, hasValue bool) error {
	switch name {
	case buildTagsFlag:
		return nil
	case "-buildvcs":
		if hasValue && value == "auto" {
			return nil
		}
	case "-race", "-msan", "-asan", "-trimpath":
	default:
		return errUnsupportedTargetQueryFlag
	}

	err := validateBooleanBuildFlag(value, hasValue)
	if err != nil {
		return fmt.Errorf("target query flag %s: %w", name, err)
	}

	return nil
}

func decodeDefaultBuildOutput(data []byte, goos string) (string, error) {
	packages, err := decodeBuildTargets(data)
	if err != nil {
		return "", err
	}

	name := ""

	for _, pkg := range packages {
		if pkg.Name != "main" {
			continue
		}

		name, err = executableTargetName(pkg)
		if err != nil {
			return "", err
		}
	}

	if len(packages) != 1 || name == "" {
		return "", nil
	}

	if goos == "windows" {
		name += ".exe"
	}

	return name, nil
}

func decodeBuildTargets(data []byte) ([]buildTargetInfo, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))

	var packages []buildTargetInfo

	for {
		var pkg buildTargetInfo

		err := decoder.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			return packages, nil
		}

		if err != nil {
			return nil, fmt.Errorf("decode build targets: %w", err)
		}

		packages = append(packages, pkg)
	}
}

func decodePreviewBuildOutputs(data []byte, goos string, directory bool) ([]string, error) {
	packages, err := decodeBuildTargets(data)
	if err != nil {
		return nil, err
	}

	if !directory {
		if len(packages) != 1 {
			return nil, errSingleFileBuildTargets
		}

		return []string{""}, nil
	}

	names := make([]string, 0, len(packages))
	for _, pkg := range packages {
		if pkg.Name != "main" {
			continue
		}

		name, nameErr := executableTargetName(pkg)
		if nameErr != nil {
			return nil, nameErr
		}

		if goos == "windows" {
			name += ".exe"
		}

		names = append(names, name)
	}

	if len(names) == 0 {
		return nil, errDirectoryBuildWithoutMain
	}

	slices.Sort(names)

	return slices.Compact(names), nil
}

func executableTargetName(pkg buildTargetInfo) (string, error) {
	if pkg.ImportPath == "command-line-arguments" {
		switch {
		case len(pkg.GoFiles) > 0:
			return strings.TrimSuffix(filepath.Base(pkg.GoFiles[0]), ".go"), nil
		case len(pkg.CgoFiles) > 0:
			return strings.TrimSuffix(filepath.Base(pkg.CgoFiles[0]), ".go"), nil
		default:
			return "", errCommandWithoutSources
		}
	}

	prefix, suffix, versioned := module.SplitPathVersion(pkg.ImportPath)
	if versioned && strings.HasPrefix(suffix, "/v") {
		return path.Base(prefix), nil
	}

	return path.Base(pkg.ImportPath), nil
}
