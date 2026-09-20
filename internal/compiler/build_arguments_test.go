package compiler

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestValidateBuildArguments(t *testing.T) {
	build := model.BuildEnvironment{BuildTags: []string{"a", "b"}, ModuleMode: "readonly", SemanticFlags: []string{"-race=true", "-buildvcs=false"}}
	for _, args := range [][]string{{"-o", "binary with spaces", "./cmd/app"}, {"-race", "--tags=b,a,a", "-mod=readonly", "-buildvcs=false", "."}, {"-p", "2", "-a", "./..."}} {
		err := ValidateBuildArguments(args, build)
		if err != nil {
			t.Fatalf("valid arguments %v: %v", args, err)
		}
	}

	for _, args := range [][]string{{"-race=false"}, {"-tags=other"}, {"-mod=mod"}, {"-o"}, {"-tags="}, {"-toolexec=other"}, {"-overlay=other"}, {"-modfile=other.mod"}, {"-C", ".."}, {"-n"}, {"-buildvcs=true"}} {
		err := ValidateBuildArguments(args, build)
		if err == nil {
			t.Fatalf("accepted override %v", args)
		}
	}
}

func TestValidateBuildArgumentsEmptyTags(t *testing.T) {
	err := ValidateBuildArguments([]string{"-tags="}, model.BuildEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
}
