package compiler

import (
	"reflect"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestRecordedBuildEnvironment(t *testing.T) {
	base := []string{"GOOS=wrong", "GOOS=also-wrong", "GOFLAGS=-tags=ambient", "CGO_ENABLED=1", "GOTOOLCHAIN=auto", "GOPROXY=off"}
	original := append([]string(nil), base...)
	build := model.BuildEnvironment{GoVersion: "go1.27.0", GOOS: "linux", GOARCH: "arm64", CGOEnabled: "0", GOARM64: "v8.0", BuildTags: []string{"z", "a"}, SemanticFlags: []string{"-race=true"}}
	env, flags, err := RecordedBuildEnvironment(base, build)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		if _, exists := values[key]; exists {
			t.Fatalf("duplicate environment key %s", key)
		}
		values[key] = value
	}
	if values["GOOS"] != "linux" || values["GOARCH"] != "arm64" || values["GOFLAGS"] != "" || values["CGO_ENABLED"] != "0" || values["GOTOOLCHAIN"] != "go1.27.0" || values["GOPROXY"] != "off" {
		t.Fatal("recorded environment not restored")
	}
	if !reflect.DeepEqual(flags, []string{"-race=true", "-tags=a,z"}) || !reflect.DeepEqual(base, original) || !reflect.DeepEqual(build.BuildTags, []string{"z", "a"}) {
		t.Fatal("flags or caller state changed")
	}
}

func TestRecordedBuildEnvironmentRequiresIdentity(t *testing.T) {
	for _, build := range []model.BuildEnvironment{{}, {GoVersion: "invalid", GOOS: "linux", GOARCH: "amd64"}, {GoVersion: "go1.27.0", GOARCH: "amd64"}} {
		env, flags, err := RecordedBuildEnvironment(nil, build)
		if err == nil || env != nil || flags != nil {
			t.Fatal("accepted incomplete build identity")
		}
	}
}
