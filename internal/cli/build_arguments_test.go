package cli

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/internal/discovery"
)

func TestParseBuildArguments(t *testing.T) {
	args := []string{"-o", "old", "--tags", "a,b", "-race", "-trimpath=false", "-mod=readonly", "-modfile", "path with spaces.mod", "-p", "2", "-o=bin/my app", "./cmd/api"}
	before := append([]string(nil), args...)

	got, err := parseBuildArguments(args)
	if err != nil {
		t.Fatal(err)
	}

	want := buildArguments{Output: "bin/my app", AnalysisFlags: []string{"-tags=a,b", "-race=true", "-trimpath=false", "-mod=readonly", "-modfile=path with spaces.mod"}, GoArgs: []string{"-p=2", "./cmd/api"}, Packages: []string{"./cmd/api"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("build arguments = %+v; want %+v", got, want)
	}

	if !reflect.DeepEqual(args, before) {
		t.Fatal("caller arguments changed")
	}

	for _, args := range [][]string{nil, {"-tags="}, {"-buildvcs=auto", "-v"}, {"main.go", "helper.go"}} {
		if _, err := parseBuildArguments(args); err != nil {
			t.Fatalf("rejected %v: %v", args, err)
		}
	}
}

func TestParseBuildArgumentsErrors(t *testing.T) {
	for _, args := range [][]string{{"-o"}, {"-o="}, {"-mod=invalid"}, {"-modfile=private-secret.txt"}, {"-race=private-secret"}, {"-overlay=private-secret"}, {"-toolexec", "private-secret"}, {"-C", "private-secret"}} {
		if _, err := parseBuildArguments(args); err == nil {
			t.Fatalf("accepted %v", args)
		} else if strings.Contains(err.Error(), "private-secret") {
			t.Fatal("diagnostic exposed argument value")
		}
	}
}

func TestBuildArgumentsApplyEffectiveFlags(t *testing.T) {
	root, _ := cliFixture(t)

	planned, err := parseBuildArguments([]string{"-tags=first", "-tags=final", "-trimpath=false", "-trimpath", "-mod=readonly", "-p=2", "."})
	if err != nil {
		t.Fatal(err)
	}

	code, err := discovery.LoadContext(t.Context(), discovery.Options{Root: root, BuildFlags: planned.AnalysisFlags, Env: []string{"GOWORK=off", "GOFLAGS="}})
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(code.EffectiveBuild.BuildTags, []string{"final"}) || !slices.Contains(code.EffectiveBuild.SemanticFlags, "-trimpath=true") {
		t.Fatal("last explicit flag did not win")
	}

	if err := compiler.ValidateBuildArguments(planned.GoArgs, code.EffectiveBuild); err != nil {
		t.Fatalf("planned flags conflict with analysis: %v", err)
	}
}
