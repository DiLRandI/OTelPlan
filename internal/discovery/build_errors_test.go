package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
)

func TestLoadBuildConfigurationErrors(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/flags\n\ngo 1.27\n"), 0o600)
	if err != nil {
		t.Fatalf("write build flag fixture: %v", err)
	}

	for _, testCase := range []struct {
		name    string
		flag    string
		message string
	}{
		{name: "missing value", flag: "-mod", message: "invalid GOFLAGS: -mod requires =value"},
		{name: "module mode", flag: "-mod=private-value", message: "invalid GOFLAGS: unsupported module mode"},
		{name: "empty manifest", flag: "-modfile=", message: "invalid GOFLAGS: empty modfile"},
		{name: "boolean", flag: "-race=private-value", message: "invalid GOFLAGS: boolean option -race"},
		{name: "overlay", flag: "-overlay=private-path", message: "unsupported GOFLAGS option -overlay"},
		{name: "unknown option", flag: "-unknown=private-value", message: "unsupported GOFLAGS option -unknown"},
		{name: "bare token", flag: "private-value", message: "invalid GOFLAGS token"},
		{name: "manifest extension", flag: "-modfile=private-path.txt",
			message: "alternate module manifest must have a .mod extension"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			code, err := discovery.Load(discovery.Options{
				Root: root, Patterns: nil, BuildTags: nil, BuildFlags: []string{testCase.flag}, CallGraph: false,
				IncludeTests: false, IncludeDependencies: false, GOOS: "", GOARCH: "", Offline: true,
				Env: []string{"GOWORK=off", "GOFLAGS="},
			})
			if err == nil || err.Error() != testCase.message {
				t.Fatalf("Load error = %v; want %q", err, testCase.message)
			}

			if code != nil {
				t.Fatal("invalid build configuration returned a code model")
			}
		})
	}
}
