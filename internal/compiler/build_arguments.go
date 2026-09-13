package compiler

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

// ValidateBuildArguments rejects overrides of the recorded analysis configuration.
func ValidateBuildArguments(args []string, build model.BuildEnvironment) error {
	semantic := map[string]string{"-race": "false", "-msan": "false", "-asan": "false", "-trimpath": "false", "-buildvcs": "auto"}
	for _, flag := range build.SemanticFlags {
		key, value, _ := strings.Cut(flag, "=")
		semantic[key] = value
	}
	for i := 0; i < len(args); i++ {
		argument := args[i]
		if !strings.HasPrefix(argument, "-") || argument == "-" {
			break
		}
		name, value, hasValue := strings.Cut(argument, "=")
		if strings.HasPrefix(name, "--") {
			name = name[1:]
		}
		if wanted, ok := semantic[name]; ok {
			if !hasValue {
				value = "true"
			}
			if value != wanted {
				return fmt.Errorf("build flag %s differs from analysis", name)
			}
			continue
		}
		switch name {
		case "-o", "-p", "-tags", "-mod":
			if !hasValue {
				i++
				if i >= len(args) {
					return fmt.Errorf("build flag %s requires a value", name)
				}
				value = args[i]
			}
			if value == "" && name != "-tags" {
				return fmt.Errorf("build flag %s requires a value", name)
			}
			if name == "-tags" && !slices.Equal(normalizedBuildTags(strings.FieldsFunc(value, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })), normalizedBuildTags(build.BuildTags)) {
				return fmt.Errorf("build tags differ from analysis")
			}
			if name == "-mod" && value != build.ModuleMode {
				return fmt.Errorf("build module mode differs from analysis")
			}
		case "-a", "-v", "-x":
			if hasValue && value != "true" && value != "false" {
				return fmt.Errorf("invalid boolean build flag")
			}
		default:
			return fmt.Errorf("unsupported build flag; compiler and source-selection overrides require analysis support")
		}
	}
	return nil
}

func normalizedBuildTags(tags []string) []string {
	result := append([]string(nil), tags...)
	slices.Sort(result)
	return slices.Compact(result)
}
