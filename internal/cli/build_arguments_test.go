package cli

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/internal/discovery"
)

func TestParseBuildArguments(t *testing.T) {
	t.Parallel()

	args := []string{
		"-o", "old", "--tags", "a,b", "-race", "-trimpath=false", "-mod=readonly", "-modfile",
		"path with spaces.mod", "-p", "2", "-o=bin/my app", "./cmd/api",
	}
	before := slices.Clone(args)

	got, err := parseBuildArguments(args)
	if err != nil {
		t.Fatal(err)
	}

	want := buildArguments{
		Output: "bin/my app",
		AnalysisFlags: []string{
			"-tags=a,b", "-race=true", "-trimpath=false", "-mod=readonly", "-modfile=path with spaces.mod",
		},
		GoArgs:   []string{"-p=2", "./cmd/api"},
		Packages: []string{"./cmd/api"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("build arguments = %+v; want %+v", got, want)
	}

	if !reflect.DeepEqual(args, before) {
		t.Fatal("caller arguments changed")
	}
}

func TestParseBuildArgumentsPackageBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "first package stops flag parsing",
			args: []string{"./cmd/api", "-unknown"}, want: []string{"./cmd/api", "-unknown"},
		},
		{name: "standalone dash is package", args: []string{"-", "-unknown"}, want: []string{"-", "-unknown"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseBuildArguments(test.args)
			if err != nil {
				t.Fatal(err)
			}

			if !slices.Equal(got.Packages, test.want) || !slices.Equal(got.GoArgs, test.want) {
				t.Fatalf("packages = %v, Go args = %v; want %v", got.Packages, got.GoArgs, test.want)
			}
		})
	}
}

func TestParseBuildArgumentsAcceptedValues(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		nil,
		{"-tags="},
		{"-buildvcs=auto"},
		{"-buildvcs=auto", "-v"},
		{"-mod=mod"},
		{"-mod=readonly"},
		{"-mod=vendor"},
		{"main.go", "helper.go"},
	} {
		_, err := parseBuildArguments(args)
		if err != nil {
			t.Errorf("parseBuildArguments(%v): %v", args, err)
		}
	}
}

func TestParseBuildArgumentsExecutionFlags(t *testing.T) {
	t.Parallel()

	got, err := parseBuildArguments([]string{"-a", "-v", "-x", "-race", "-trimpath=false", "./cmd/api"})
	if err != nil {
		t.Fatal(err)
	}

	wantGoArgs := []string{"-a=true", "-v=true", "-x=true", "./cmd/api"}
	if !slices.Equal(got.GoArgs, wantGoArgs) {
		t.Fatalf("Go args = %v", got.GoArgs)
	}

	wantAnalysisFlags := []string{"-race=true", "-trimpath=false"}
	if !slices.Equal(got.AnalysisFlags, wantAnalysisFlags) {
		t.Fatalf("analysis flags = %v", got.AnalysisFlags)
	}
}

func TestParseBuildArgumentsErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{name: "missing output value", args: []string{"-o"}, message: "build flag -o requires a value"},
		{name: "empty output value", args: []string{"-o="}, message: "build flag -o requires a value"},
		{
			name: "missing value after valid flags", args: []string{"-tags=accepted", "-o"},
			message: "build flag -o requires a value",
		},
		{name: "invalid boolean", args: []string{"-race=private-secret"}, message: "invalid boolean build flag -race"},
		{name: "invalid module mode", args: []string{"-mod=private-secret"}, message: "invalid build module mode"},
		{
			name:    "invalid module file",
			args:    []string{"-modfile=private-secret.txt"},
			message: "build module file requires a .mod extension",
		},
		{
			name:    "unsupported flag",
			args:    []string{"-overlay=private-secret"},
			message: "unsupported build flag; source-selection and compiler overrides require analysis support",
		},
		{
			name:    "unsupported compiler tool",
			args:    []string{"-toolexec", "private-secret"},
			message: "unsupported build flag; source-selection and compiler overrides require analysis support",
		},
		{
			name:    "unsupported directory change",
			args:    []string{"-C", "private-secret"},
			message: "unsupported build flag; source-selection and compiler overrides require analysis support",
		},
		{name: "missing tags value", args: []string{"-tags"}, message: "build flag -tags requires a value"},
		{name: "missing module file value", args: []string{"-modfile"}, message: "build flag -modfile requires a value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			args := slices.Clone(test.args)
			before := slices.Clone(args)

			first, err := parseBuildArguments(args)
			if err == nil {
				t.Fatal("parseBuildArguments succeeded")
			}

			if err.Error() != test.message {
				t.Fatalf("error = %q; want %q", err, test.message)
			}

			if strings.Contains(err.Error(), "private-secret") {
				t.Fatal("diagnostic exposed argument value")
			}

			var zero buildArguments
			if !reflect.DeepEqual(first, zero) {
				t.Fatalf("partial result = %+v; want zero value", first)
			}

			if !reflect.DeepEqual(args, before) {
				t.Fatal("caller arguments changed")
			}
		})
	}
}

func TestParseBuildArgumentsErrorsHaveStableCauses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "missing value", args: []string{"-o"}},
		{name: "invalid boolean", args: []string{"-race=private-secret"}},
		{name: "invalid module mode", args: []string{"-mod=private-secret"}},
		{name: "invalid module file", args: []string{"-modfile=private-secret.txt"}},
		{name: "unsupported flag", args: []string{"-overlay=private-secret"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, firstErr := parseBuildArguments(slices.Clone(test.args))

			_, secondErr := parseBuildArguments(slices.Clone(test.args))
			if firstErr == nil || secondErr == nil {
				t.Fatal("parseBuildArguments succeeded")
			}

			cause := firstErr
			for err := errors.Unwrap(cause); err != nil; err = errors.Unwrap(cause) {
				cause = err
			}

			if !errors.Is(secondErr, cause) {
				t.Fatalf("error cause is not stable: first %v, second %v", firstErr, secondErr)
			}
		})
	}
}

func TestBuildArgumentsApplyEffectiveFlags(t *testing.T) {
	t.Parallel()

	root, _ := cliFixture(t)

	planned, err := parseBuildArguments([]string{
		"-tags=first", "-tags=final", "-trimpath=false", "-trimpath", "-mod=readonly", "-p=2", ".",
	})
	if err != nil {
		t.Fatal(err)
	}

	var options discovery.Options

	options.Root = root
	options.BuildFlags = planned.AnalysisFlags
	options.Env = []string{"GOWORK=off", "GOFLAGS="}

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(code.EffectiveBuild.BuildTags, []string{"final"}) ||
		!slices.Contains(code.EffectiveBuild.SemanticFlags, "-trimpath=true") {
		t.Fatal("last explicit flag did not win")
	}

	err = compiler.ValidateBuildArguments(planned.GoArgs, code.EffectiveBuild)
	if err != nil {
		t.Fatalf("planned flags conflict with analysis: %v", err)
	}
}
