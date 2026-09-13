package compiler

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestBuildResolvedFailureCleanup(t *testing.T) {
	source, parent := t.TempDir(), t.TempDir()
	manifest := []byte("module example.com/app\n\ngo 1.27.0\n")
	if err := os.WriteFile(filepath.Join(source, "go.mod"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	backend, _ := otelc.Identity(otelc.SupportedVersion)
	code := &model.CodeModel{ModuleRoot: source, Modules: []model.ModuleInfo{{Main: true, Dir: source, Path: "example.com/app"}}, EffectiveBuild: model.BuildEnvironment{GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, ModuleMode: "readonly"}}
	request := ResolvedBuildRequest{Code: code, Backend: backend, Executable: filepath.Join(parent, "missing-backend"), RuntimeVersion: "test", Parent: parent, Env: os.Environ(), Offline: true}
	for _, change := range []func(*ResolvedBuildRequest){
		func(r *ResolvedBuildRequest) { r.GoArgs = []string{"-o", "elsewhere"} },
		func(r *ResolvedBuildRequest) { r.Code = nil },
		func(r *ResolvedBuildRequest) { r.WorkingDir = filepath.Join(source, "missing") },
		func(r *ResolvedBuildRequest) {},
	} {
		invalid := request
		change(&invalid)
		if _, err := BuildResolved(t.Context(), invalid); err == nil {
			t.Fatal("accepted invalid build request")
		}
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) != 0 {
			t.Fatal("failed build left temporary output")
		}
	}
	data, err := os.ReadFile(filepath.Join(source, "go.mod"))
	if err != nil || string(data) != string(manifest) {
		t.Fatal("failed build changed source module")
	}
}
