package cli

import (
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestProtectedBuildOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	config := filepath.Join(root, "policy", "otelplan.yaml")
	modFile := filepath.Join(root, "alternate", "selected.mod")
	workspaceFile := filepath.Join(root, "workspace", "selected.work")

	var opts options

	opts.root = root
	opts.config = filepath.Join("policy", "otelplan.yaml")
	code := new(model.CodeModel)
	code.EffectiveBuild.ModFile = modFile
	code.WorkspaceFile = workspaceFile

	tests := []struct {
		name        string
		destination string
		want        bool
	}{
		{name: "empty output is allowed", destination: "", want: false},
		{name: "source file anywhere", destination: filepath.Join(root, "nested", "handler.go"), want: true},
		{name: "module manifest anywhere", destination: filepath.Join(root, "nested", "go.mod"), want: true},
		{name: "module sum anywhere", destination: filepath.Join(root, "nested", "go.sum"), want: true},
		{name: "workspace manifest anywhere", destination: filepath.Join(root, "nested", "go.work"), want: true},
		{name: "workspace sum anywhere", destination: filepath.Join(root, "nested", "go.work.sum"), want: true},
		{name: "relative configured policy", destination: config, want: true},
		{name: "project lock at root", destination: filepath.Join(root, "otelplan.lock"), want: true},
		{name: "lock name in subdirectory", destination: filepath.Join(root, "nested", "otelplan.lock"), want: false},
		{name: "alternate module manifest", destination: modFile, want: true},
		{name: "workspace manifest", destination: workspaceFile, want: true},
		{name: "ordinary output", destination: filepath.Join(root, "bin", "service"), want: false},
		{name: "same name in another directory", destination: filepath.Join(root, "other", "policy", "otelplan.yaml"),
			want: false},
		{name: "normalized dot segments", destination: filepath.Join(root, "tmp", "..", "otelplan.lock"), want: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := protectedBuildOutput(testCase.destination, opts, code); got != testCase.want {
				t.Errorf("protectedBuildOutput(%q) = %t, want %t", testCase.destination, got, testCase.want)
			}
		})
	}

	absoluteConfigOptions := opts
	absoluteConfigOptions.config = config

	if !protectedBuildOutput(config, absoluteConfigOptions, code) {
		t.Error("protectedBuildOutput did not protect the absolute configured policy path")
	}
}
