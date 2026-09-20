package cli

import (
	"fmt"
	"strings"
)

type buildArguments struct {
	Output        string
	AnalysisFlags []string
	GoArgs        []string
	Packages      []string
}

func parseBuildArguments(args []string) (buildArguments, error) {
	result := buildArguments{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			result.Packages = append([]string(nil), args[i:]...)
			result.GoArgs = append(result.GoArgs, result.Packages...)
			return result, nil
		}
		name, value, hasValue := strings.Cut(arg, "=")
		if strings.HasPrefix(name, "--") {
			name = name[1:]
		}
		switch name {
		case "-o", "-p", "-tags", "-mod", "-modfile":
			if !hasValue {
				i++
				if i == len(args) {
					return buildArguments{}, fmt.Errorf("build flag %s requires a value", name)
				}
				value = args[i]
			}
			if value == "" && name != "-tags" {
				return buildArguments{}, fmt.Errorf("build flag %s requires a value", name)
			}
			switch name {
			case "-o":
				result.Output = value
			case "-p":
				result.GoArgs = append(result.GoArgs, name+"="+value)
			case "-tags":
				result.AnalysisFlags = append(result.AnalysisFlags, name+"="+value)
			case "-mod":
				if value != "mod" && value != "readonly" && value != "vendor" {
					return buildArguments{}, fmt.Errorf("invalid build module mode")
				}
				result.AnalysisFlags = append(result.AnalysisFlags, name+"="+value)
			case "-modfile":
				if !strings.HasSuffix(value, ".mod") {
					return buildArguments{}, fmt.Errorf("build module file requires a .mod extension")
				}
				result.AnalysisFlags = append(result.AnalysisFlags, name+"="+value)
			}
		case "-race", "-msan", "-asan", "-trimpath", "-buildvcs", "-a", "-v", "-x":
			if !hasValue {
				value = "true"
			}
			if value != "true" && value != "false" && (name != "-buildvcs" || value != "auto") {
				return buildArguments{}, fmt.Errorf("invalid boolean build flag %s", name)
			}
			token := name + "=" + value
			if name != "-a" && name != "-v" && name != "-x" {
				result.AnalysisFlags = append(result.AnalysisFlags, token)
			} else {
				result.GoArgs = append(result.GoArgs, token)
			}
		default:
			return buildArguments{}, fmt.Errorf("unsupported build flag; source-selection and compiler overrides require analysis support")
		}
	}
	return result, nil
}
