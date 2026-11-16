package cli

import (
	"os"
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

func TestProtectedBuildOutputThroughDirectoryAliases(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "project-alias")

	err := os.Symlink(root, alias)
	if err != nil {
		t.Fatal(err)
	}

	var opts options

	opts.root, opts.config = root, "policy.yaml"
	code := new(model.CodeModel)
	code.EffectiveBuild.ModFile = filepath.Join(root, "selected.mod")
	code.WorkspaceFile = filepath.Join(root, "selected.work")

	for _, name := range []string{"policy.yaml", "selected.mod", "selected.work"} {
		err := os.WriteFile(filepath.Join(root, name), []byte("owned metadata"), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "policy.yaml", want: true},
		{name: "selected.mod", want: true},
		{name: "selected.work", want: true},
		{name: "otelplan.lock", want: true},
		{name: "ordinary-output", want: false},
		{name: "new/ordinary-output", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := protectedBuildOutput(filepath.Join(alias, test.name), opts, code); got != test.want {
				t.Fatalf("aliased output %s protection=%t; want %t", test.name, got, test.want)
			}

			aliasOptions := opts
			aliasOptions.root = alias

			if got := protectedBuildOutput(filepath.Join(root, test.name), aliasOptions, code); got != test.want {
				t.Fatalf("aliased root output %s protection=%t; want %t", test.name, got, test.want)
			}
		})
	}
}

func TestProtectedBuildOutputFollowsSelectedPolicySymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "actual-policy.yaml")

	err := os.WriteFile(target, []byte("owned policy"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Symlink(target, filepath.Join(root, "selected.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	var opts options

	opts.root, opts.config = root, "selected.yaml"
	code := new(model.CodeModel)

	if !protectedBuildOutput(target, opts, code) {
		t.Fatal("build output can replace the selected policy symlink target")
	}
}
