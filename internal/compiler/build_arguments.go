package compiler

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const (
	buildFlagTrue  = "true"
	buildFlagFalse = "false"
	buildTagsFlag  = "-tags"
)

var (
	errBuildFlagMismatch        = errors.New("differs from analysis")
	errBuildFlagValueRequired   = errors.New("requires a value")
	errBuildTagsMismatch        = errors.New("build tags differ from analysis")
	errBuildModuleModeMismatch  = errors.New("build module mode differs from analysis")
	errInvalidBooleanBuildFlag  = errors.New("invalid boolean build flag")
	errUnsupportedBuildOverride = errors.New(
		"unsupported build flag; compiler and source-selection overrides require analysis support",
	)
)

// ValidateBuildArguments rejects overrides of the recorded analysis configuration.
func ValidateBuildArguments(args []string, build model.BuildEnvironment) error {
	semantic := semanticBuildFlags(build.SemanticFlags)

	for len(args) > 0 {
		argument := args[0]
		args = args[1:]

		if !strings.HasPrefix(argument, "-") || argument == "-" {
			break
		}

		name, value, hasValue := strings.Cut(argument, "=")
		if strings.HasPrefix(name, "--") {
			name = name[1:]
		}

		if wanted, ok := semantic[name]; ok {
			if !hasValue {
				value = buildFlagTrue
			}

			if value != wanted {
				return fmt.Errorf("build flag %s %w", name, errBuildFlagMismatch)
			}

			continue
		}

		var err error

		value, args, err = buildArgumentValue(name, value, hasValue, args)
		if err != nil {
			return err
		}

		err = validateBuildFlag(name, value, hasValue, build)
		if err != nil {
			return err
		}
	}

	return nil
}

func semanticBuildFlags(flags []string) map[string]string {
	semantic := map[string]string{
		"-race": buildFlagFalse, "-msan": buildFlagFalse, "-asan": buildFlagFalse,
		"-trimpath": buildFlagFalse, "-buildvcs": "auto",
	}

	for _, flag := range flags {
		key, value, _ := strings.Cut(flag, "=")
		semantic[key] = value
	}

	return semantic
}

func buildArgumentValue(name, value string, hasValue bool, remaining []string) (string, []string, error) {
	switch name {
	case "-o", "-p", buildTagsFlag, "-mod":
		if !hasValue {
			if len(remaining) == 0 {
				return "", remaining, fmt.Errorf("build flag %s %w", name, errBuildFlagValueRequired)
			}

			value, remaining = remaining[0], remaining[1:]
		}

		if value == "" && name != buildTagsFlag {
			return "", remaining, fmt.Errorf("build flag %s %w", name, errBuildFlagValueRequired)
		}
	}

	return value, remaining, nil
}

func validateBuildFlag(name, value string, hasValue bool, build model.BuildEnvironment) error {
	switch name {
	case "-o", "-p":
		return nil
	case buildTagsFlag:
		tags := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
		if !slices.Equal(normalizedBuildTags(tags), normalizedBuildTags(build.BuildTags)) {
			return errBuildTagsMismatch
		}
	case "-mod":
		if value != build.ModuleMode {
			return errBuildModuleModeMismatch
		}
	case "-a", "-v", "-x":
		return validateBooleanBuildFlag(value, hasValue)
	default:
		return errUnsupportedBuildOverride
	}

	return nil
}

func validateBooleanBuildFlag(value string, hasValue bool) error {
	if hasValue && value != buildFlagTrue && value != buildFlagFalse {
		return errInvalidBooleanBuildFlag
	}

	return nil
}

func normalizedBuildTags(tags []string) []string {
	result := append([]string(nil), tags...)
	slices.Sort(result)

	return slices.Compact(result)
}
