package cli

import (
	"errors"
	"fmt"
	"strings"
)

const buildTagsFlag = "-tags"

var (
	errBuildFlagValueMissing = errors.New("requires a value")
	errBuildModuleMode       = errors.New("invalid build module mode")
	errBuildModuleFile       = errors.New("build module file requires a .mod extension")
	errBooleanBuildFlag      = errors.New("invalid boolean build flag")
	errUnsupportedBuildFlag  = errors.New(
		"unsupported build flag; source-selection and compiler overrides require analysis support",
	)
)

type buildArguments struct {
	Output        string
	AnalysisFlags []string
	GoArgs        []string
	Packages      []string
}

func parseBuildArguments(args []string) (buildArguments, error) {
	var result, empty buildArguments

	remaining := args

	for len(remaining) > 0 {
		arg := remaining[0]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			result.Packages = append([]string(nil), remaining...)
			result.GoArgs = append(result.GoArgs, result.Packages...)

			return result, nil
		}

		remaining = remaining[1:]
		name, value, hasValue := splitBuildFlag(arg)

		switch name {
		case "-o", "-p", buildTagsFlag, "-mod", "-modfile":
			parsedValue, tail, err := buildFlagValue(name, value, hasValue, remaining)
			if err != nil {
				return empty, err
			}

			remaining = tail

			err = result.appendValueBuildFlag(name, parsedValue)
			if err != nil {
				return empty, err
			}
		case "-race", "-msan", "-asan", "-trimpath", "-buildvcs", "-a", "-v", "-x":
			err := result.appendBooleanBuildFlag(name, value, hasValue)
			if err != nil {
				return empty, err
			}
		default:
			return empty, errUnsupportedBuildFlag
		}
	}

	return result, nil
}

func splitBuildFlag(argument string) (string, string, bool) {
	name, value, hasValue := strings.Cut(argument, "=")
	if strings.HasPrefix(name, "--") {
		name = name[1:]
	}

	return name, value, hasValue
}

func buildFlagValue(name, value string, hasValue bool, remaining []string) (string, []string, error) {
	if !hasValue {
		if len(remaining) == 0 {
			return "", remaining, fmt.Errorf("build flag %s %w", name, errBuildFlagValueMissing)
		}

		value, remaining = remaining[0], remaining[1:]
	}

	if value == "" && name != buildTagsFlag {
		return "", remaining, fmt.Errorf("build flag %s %w", name, errBuildFlagValueMissing)
	}

	return value, remaining, nil
}

func (result *buildArguments) appendValueBuildFlag(name, value string) error {
	switch name {
	case "-o":
		result.Output = value
	case "-p":
		result.GoArgs = append(result.GoArgs, name+"="+value)
	case buildTagsFlag:
		result.AnalysisFlags = append(result.AnalysisFlags, name+"="+value)
	case "-mod":
		if value != "mod" && value != "readonly" && value != "vendor" {
			return errBuildModuleMode
		}

		result.AnalysisFlags = append(result.AnalysisFlags, name+"="+value)
	case "-modfile":
		if !strings.HasSuffix(value, ".mod") {
			return errBuildModuleFile
		}

		result.AnalysisFlags = append(result.AnalysisFlags, name+"="+value)
	}

	return nil
}

func (result *buildArguments) appendBooleanBuildFlag(name, value string, hasValue bool) error {
	if !hasValue {
		value = "true"
	}

	if value != "true" && value != "false" && (name != "-buildvcs" || value != "auto") {
		return fmt.Errorf("%w %s", errBooleanBuildFlag, name)
	}

	token := name + "=" + value

	switch name {
	case "-a", "-v", "-x":
		result.GoArgs = append(result.GoArgs, token)
	default:
		result.AnalysisFlags = append(result.AnalysisFlags, token)
	}

	return nil
}
