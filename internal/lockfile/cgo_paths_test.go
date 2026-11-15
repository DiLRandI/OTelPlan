package lockfile_test

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestGraphCGOCompilerPaths(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"CC", "CXX"} {
		for _, scenario := range []string{
			"shared external compiler", "quoted external compiler", "different external compilers", "project compiler",
		} {
			t.Run(field+"/"+scenario, func(t *testing.T) {
				t.Parallel()

				first, second := graphFixture(t), graphFixture(t)
				first.EffectiveBuild.CGOEnabled, second.EffectiveBuild.CGOEnabled = "1", "1"
				left := first.ModuleRoot + "-tools/compiler"
				right, equal := left, true

				switch scenario {
				case "quoted external compiler":
					left = strconv.Quote(first.ModuleRoot+" tools/compiler") + " -g"
					right = left
				case "different external compilers":
					right, equal = second.ModuleRoot+"-tools/compiler", false
				case "project compiler":
					left = strconv.Quote(filepath.Join(first.ModuleRoot, "tool chain", "compiler")) + " -g"
					right = strconv.Quote(filepath.Join(second.ModuleRoot, "tool chain", "compiler")) + " -g"
				}

				setGraphCompiler(first, field, left)
				setGraphCompiler(second, field, right)

				before, err := lockfile.GraphDigest(first)
				if err != nil {
					t.Fatal(err)
				}

				after, err := lockfile.GraphDigest(second)
				if err != nil {
					t.Fatal(err)
				}

				if (before == after) != equal {
					t.Fatalf("compiler fingerprint equality=%t, want %t", before == after, equal)
				}
			})
		}
	}
}

func setGraphCompiler(code *model.CodeModel, field, command string) {
	if field == "CC" {
		code.EffectiveBuild.CC = command
	} else {
		code.EffectiveBuild.CXX = command
	}
}

func TestGraphCGOFlagPathsAndLiterals(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"CGOCFLAGS", "CGOCPPFLAGS", "CGOCXXFLAGS", "CGOLDFLAGS", "CGOFFLAGS"} {
		for _, scenario := range []string{"external path", "different external paths", "project path", "literal value"} {
			t.Run(field+"/"+scenario, func(t *testing.T) {
				t.Parallel()

				first, second := graphFixture(t), graphFixture(t)
				first.EffectiveBuild.CGOEnabled, second.EffectiveBuild.CGOEnabled = "1", "1"
				left := "-I" + first.ModuleRoot + "-headers"
				right, equal := left, true

				switch scenario {
				case "different external paths":
					right, equal = "-I"+second.ModuleRoot+"-headers", false
				case "project path":
					left = strconv.Quote("-I" + filepath.Join(first.ModuleRoot, "header files"))
					right = strconv.Quote("-I" + filepath.Join(second.ModuleRoot, "header files"))
				case "literal value":
					left, right, equal = "-DVALUE="+first.ModuleRoot, "-DVALUE="+second.ModuleRoot, false
				}

				*graphCGOInput(t, first, field), *graphCGOInput(t, second, field) = left, right

				before, err := lockfile.GraphDigest(first)
				if err != nil {
					t.Fatal(err)
				}

				after, err := lockfile.GraphDigest(second)
				if err != nil {
					t.Fatal(err)
				}

				if (before == after) != equal {
					t.Fatalf("CGO configuration fingerprint equality=%t, want %t", before == after, equal)
				}
			})
		}
	}
}

func graphCGOInput(t *testing.T, code *model.CodeModel, field string) *string {
	t.Helper()

	switch field {
	case "CC":
		return &code.EffectiveBuild.CC
	case "CXX":
		return &code.EffectiveBuild.CXX
	case "CGOCFLAGS":
		return &code.EffectiveBuild.CGOCFLAGS
	case "CGOCPPFLAGS":
		return &code.EffectiveBuild.CGOCPPFLAGS
	case "CGOCXXFLAGS":
		return &code.EffectiveBuild.CGOCXXFLAGS
	case "CGOLDFLAGS":
		return &code.EffectiveBuild.CGOLDFLAGS
	case "CGOFFLAGS":
		return &code.EffectiveBuild.CGOFFLAGS
	default:
		t.Fatalf("unknown CGO test field %s", field)

		return nil
	}
}

func TestGraphCGOQuotedPathsAndOpaqueValues(t *testing.T) {
	t.Parallel()

	for _, scenario := range []struct {
		name, value string
		equal       bool
	}{
		{name: "quoted command", value: "\t '@root@/tool chain/compiler'  --sysroot '@root@/sdk path' \n", equal: true},
		{name: "wrapper argument", value: "cache @root@/bin/compiler", equal: true},
		{name: "separate include", value: "compiler -I '@root@/header files'", equal: true},
		{name: "empty word", value: "compiler -I '' '-I@root@/include'", equal: true},
		{name: "no unescaping", value: "'@root@/tool\\name/compiler'", equal: true},
		{name: "literal macro", value: "compiler -DVALUE=@root@/literal", equal: false},
		{name: "opaque compound option", value: "compiler -fdebug-prefix-map=@root@=result", equal: false},
		{name: "unterminated quote", value: "'@root@/compiler", equal: false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			first, second := graphFixture(t), graphFixture(t)
			first.EffectiveBuild.CGOEnabled, second.EffectiveBuild.CGOEnabled = "1", "1"
			first.EffectiveBuild.CC = strings.ReplaceAll(scenario.value, "@root@", first.ModuleRoot)
			second.EffectiveBuild.CC = strings.ReplaceAll(scenario.value, "@root@", second.ModuleRoot)

			before, err := lockfile.GraphDigest(first)
			if err != nil {
				t.Fatal(err)
			}

			after, err := lockfile.GraphDigest(second)
			if err != nil {
				t.Fatal(err)
			}

			if (before == after) != scenario.equal {
				t.Fatalf("quoted configuration fingerprint equality=%t, want %t", before == after, scenario.equal)
			}
		})
	}
}

func TestGraphCGOLiteralCannotAliasProjectPath(t *testing.T) {
	t.Parallel()

	first, second := graphFixture(t), graphFixture(t)
	first.EffectiveBuild.CGOEnabled, second.EffectiveBuild.CGOEnabled = "1", "1"
	first.EffectiveBuild.CC = filepath.Join(first.ModuleRoot, "compiler")
	second.EffectiveBuild.CC = "${PROJECT}/compiler"

	before, err := lockfile.GraphDigest(first)
	if err != nil {
		t.Fatal(err)
	}

	after, err := lockfile.GraphDigest(second)
	if err != nil {
		t.Fatal(err)
	}

	if before == after {
		t.Fatal("literal compiler name aliased a normalized project path")
	}
}

func TestGraphCGORejectsNullEnvironmentValues(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"CC", "CXX", "CGOCFLAGS", "CGOCPPFLAGS", "CGOCXXFLAGS", "CGOLDFLAGS", "CGOFFLAGS"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			code := graphFixture(t)
			code.EffectiveBuild.CGOEnabled = "1"
			*graphCGOInput(t, code, field) = "private-value\x00project\x00/compiler"

			digest, err := lockfile.GraphDigest(code)
			if err == nil || digest != "" || strings.Contains(err.Error(), "private-value") {
				t.Fatalf("invalid environment input was accepted or exposed: %q, %v", digest, err)
			}
		})
	}
}
