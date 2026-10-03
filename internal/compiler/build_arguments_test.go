package compiler_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestValidateBuildArguments(t *testing.T) {
	t.Parallel()

	build := new(model.BuildEnvironment)
	build.BuildTags = []string{"a", "b"}
	build.ModuleMode = "readonly"
	build.SemanticFlags = []string{"-race=true", "-buildvcs=false"}

	for _, args := range [][]string{
		{"-o", "binary with spaces", "./cmd/app"},
		{"-race", "--tags=b,a,a", "-mod=readonly", "-buildvcs=false", "."},
		{"-p", "2", "-a", "./..."},
	} {
		err := compiler.ValidateBuildArguments(args, *build)
		if err != nil {
			t.Fatalf("valid arguments %v: %v", args, err)
		}
	}

	for _, args := range [][]string{
		{"-race=false"}, {"-tags=other"}, {"-mod=mod"}, {"-o"}, {"-tags="},
		{"-toolexec=other"}, {"-overlay=other"}, {"-modfile=other.mod"}, {"-C", ".."},
		{"-n"}, {"-buildvcs=true"},
	} {
		err := compiler.ValidateBuildArguments(args, *build)
		if err == nil {
			t.Fatalf("accepted override %v", args)
		}
	}
}

func TestValidateBuildArgumentsEmptyTags(t *testing.T) {
	t.Parallel()

	build := new(model.BuildEnvironment)

	err := compiler.ValidateBuildArguments([]string{"-tags="}, *build)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBuildArgumentBoundariesAndErrorMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{name: "missing output", args: []string{"-o"}, message: "build flag -o requires a value"},
		{name: "missing tags", args: []string{"-tags"}, message: "build flag -tags requires a value"},
		{name: "empty output", args: []string{"-o="}, message: "build flag -o requires a value"},
		{name: "empty concurrency", args: []string{"--p="}, message: "build flag -p requires a value"},
		{name: "invalid boolean", args: []string{"-a=invalid"}, message: "invalid boolean build flag"},
		{name: "changed semantic", args: []string{"-race"}, message: "build flag -race differs from analysis"},
		{name: "package terminates flags", args: []string{".", "-race"}, message: ""},
		{name: "dash terminates flags", args: []string{"-", "-race"}, message: ""},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			build := new(model.BuildEnvironment)
			original := slices.Clone(testCase.args)

			err := compiler.ValidateBuildArguments(testCase.args, *build)
			if !slices.Equal(original, testCase.args) {
				t.Fatal("validation changed caller-owned arguments")
			}

			if testCase.message == "" {
				if err != nil {
					t.Fatal(err)
				}

				return
			}

			if err == nil || err.Error() != testCase.message {
				t.Fatalf("error = %v; want %s", err, testCase.message)
			}
		})
	}
}

func TestBuildArgumentErrorsShareStableCause(t *testing.T) {
	t.Parallel()

	build := new(model.BuildEnvironment)
	first := compiler.ValidateBuildArguments([]string{"-o"}, *build)
	second := compiler.ValidateBuildArguments([]string{"-p="}, *build)

	cause := errors.Unwrap(first)
	if cause == nil || !errors.Is(second, cause) {
		t.Fatal("missing-value failures do not retain an inspectable shared cause")
	}
}

func TestBuildArgumentDiagnosticsOmitFlagValues(t *testing.T) {
	t.Parallel()

	build := new(model.BuildEnvironment)

	const sensitive = "private-value-must-not-be-printed"

	for _, flag := range []string{"-race", "-tags", "-mod", "-a", "-overlay"} {
		err := compiler.ValidateBuildArguments([]string{flag + "=" + sensitive}, *build)
		if err == nil || strings.Contains(err.Error(), sensitive) {
			t.Fatal("invalid build input was accepted or exposed its value", err)
		}
	}
}

func TestBuildArgumentTagNormalizationDoesNotMutateInput(t *testing.T) {
	t.Parallel()

	build := new(model.BuildEnvironment)
	build.BuildTags = []string{"b", "a", "a"}
	original := slices.Clone(build.BuildTags)

	err := compiler.ValidateBuildArguments([]string{"-tags=a,b"}, *build)
	if err != nil || !slices.Equal(original, build.BuildTags) {
		t.Fatal("equivalent tags failed or modified the analyzed build input", err)
	}
}
