package compiler_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestRecordedBuildEnvironment(t *testing.T) {
	t.Parallel()

	base := []string{
		"GOOS=wrong", "GOOS=also-wrong", "GOFLAGS=-tags=ambient",
		"CGO_ENABLED=1", "GOTOOLCHAIN=auto", "GOPROXY=off",
	}
	original := slices.Clone(base)
	build := new(model.BuildEnvironment)
	build.GoVersion, build.GOOS, build.GOARCH = "go1.27.0", "linux", "arm64"
	build.CGOEnabled, build.GOARM64 = "0", "v8.0"
	build.BuildTags, build.SemanticFlags = []string{"z", "a"}, []string{"-race=true"}

	env, flags, err := compiler.RecordedBuildEnvironment(base, *build)
	if err != nil {
		t.Fatal(err)
	}

	values := environmentValues(t, env)

	expected := map[string]string{
		"GOOS": "linux", "GOARCH": "arm64", "GOFLAGS": "", "CGO_ENABLED": "0",
		"GOTOOLCHAIN": "go1.27.0", "GOPROXY": "off", "GOARM64": "v8.0",
	}
	for key, value := range expected {
		if values[key] != value {
			t.Fatalf("recorded environment setting %s was not restored", key)
		}
	}

	if !slices.Equal(flags, []string{"-race=true", "-tags=a,z"}) || !slices.Equal(base, original) ||
		!slices.Equal(build.BuildTags, []string{"z", "a"}) {
		t.Fatal("flags or caller state changed")
	}
}

func environmentValues(t *testing.T, env []string) map[string]string {
	t.Helper()

	values := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		if _, exists := values[key]; exists {
			t.Fatalf("duplicate environment key %s", key)
		}

		values[key] = value
	}

	return values
}

func TestRecordedBuildEnvironmentRequiresIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		goos    string
		goarch  string
	}{
		{name: "empty", version: "", goos: "", goarch: ""},
		{name: "invalid version", version: "invalid", goos: "linux", goarch: "amd64"},
		{name: "missing OS", version: "go1.27.0", goos: "", goarch: "amd64"},
		{name: "missing architecture", version: "go1.27.0", goos: "linux", goarch: ""},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			build := new(model.BuildEnvironment)
			build.GoVersion, build.GOOS, build.GOARCH = testCase.version, testCase.goos, testCase.goarch

			env, flags, err := compiler.RecordedBuildEnvironment(nil, *build)
			if err == nil || env != nil || flags != nil {
				t.Fatal("accepted incomplete build identity")
			}
		})
	}
}

func TestRecordedBuildEnvironmentErrorsShareIdentity(t *testing.T) {
	t.Parallel()

	build := new(model.BuildEnvironment)
	_, _, first := compiler.RecordedBuildEnvironment(nil, *build)
	build.GoVersion = "invalid"

	_, _, second := compiler.RecordedBuildEnvironment(nil, *build)
	if first == nil || !errors.Is(second, first) || first.Error() != "complete analyzed Go environment is required" {
		t.Fatal("incomplete identities lost their stable cause or meaningful message")
	}
}

func TestRecordedCompilerSettingsOverrideAmbientValues(t *testing.T) {
	t.Parallel()

	build := new(model.BuildEnvironment)
	build.GoVersion, build.GOOS, build.GOARCH = "go1.27.0", "linux", "amd64"
	build.CC, build.CXX = "recorded compiler", "recorded C++ compiler"
	build.CGOCFLAGS, build.CGOCPPFLAGS = "recorded C flags", "recorded preprocessor flags"
	build.CGOCXXFLAGS, build.CGOLDFLAGS = "recorded C++ flags", "recorded linker flags"
	build.CGOFFLAGS = "recorded Fortran flags"

	base := []string{"CC=ambient", "CXX=ambient", "CGO_CFLAGS=ambient", "CGO_CPPFLAGS=ambient",
		"CGO_CXXFLAGS=ambient", "CGO_LDFLAGS=ambient", "CGO_FFLAGS=ambient"}

	env, _, err := compiler.RecordedBuildEnvironment(base, *build)
	if err != nil {
		t.Fatal(err)
	}

	values := environmentValues(t, env)

	expected := map[string]string{
		"CC": build.CC, "CXX": build.CXX, "CGO_CFLAGS": build.CGOCFLAGS, "CGO_CPPFLAGS": build.CGOCPPFLAGS,
		"CGO_CXXFLAGS": build.CGOCXXFLAGS, "CGO_LDFLAGS": build.CGOLDFLAGS, "CGO_FFLAGS": build.CGOFFLAGS,
	}
	for key, value := range expected {
		if values[key] != value {
			t.Fatalf("recorded compiler setting %s was not restored", key)
		}
	}
}
