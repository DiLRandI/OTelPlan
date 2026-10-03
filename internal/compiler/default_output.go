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
	"strings"

	"golang.org/x/mod/module"
)

const targetQueryVendorMode = "vendor"

var (
	errCommandWithoutSources        = errors.New("command has no source files")
	errUnsupportedTargetQueryFlag   = errors.New("unsupported target query flag")
	errTargetQueryFlagValueRequired = errors.New("target query flag requires a value")
	errInvalidTargetQueryModuleMode = errors.New("unsupported target query module mode")
)

type buildTargetInfo struct {
	Name       string   `json:"Name"`
	ImportPath string   `json:"ImportPath"`
	GoFiles    []string `json:"GoFiles"`
	CgoFiles   []string `json:"CgoFiles"`
}

func defaultBuildOutput(ctx context.Context, dir string, env, flags, targets []string,
	goos, moduleMode string) (string, error) {
	err := validateTargetQueryFlags(flags, moduleMode)
	if err != nil {
		return "", err
	}

	command := exec.CommandContext(ctx, "go", "list", "-json")
	command.Args = append(command.Args, "-mod="+moduleMode)
	command.Args = append(command.Args, flags...)
	command.Args = append(command.Args, "--")
	command.Args = append(command.Args, targets...)
	command.Dir, command.Env = dir, env

	data, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read build targets: %w", err)
	}

	return decodeDefaultBuildOutput(data, goos)
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
	decoder := json.NewDecoder(bytes.NewReader(data))
	count, name := 0, ""

	for {
		var pkg buildTargetInfo

		err := decoder.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return "", fmt.Errorf("decode build targets: %w", err)
		}

		count++

		if pkg.Name != "main" {
			continue
		}

		name, err = executableTargetName(pkg)
		if err != nil {
			return "", err
		}
	}

	if count != 1 || name == "" {
		return "", nil
	}

	if goos == "windows" {
		name += ".exe"
	}

	return name, nil
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
