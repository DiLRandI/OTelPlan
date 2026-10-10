package discovery_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
)

func TestLoadNormalizesBooleanAndDiagnosticFlags(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, contents := range map[string]string{
		"go.mod": "module example.com/flags\n\ngo 1.27\n",
		"app.go": "package flags\nfunc Run() {}\n",
	} {
		err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600)
		if err != nil {
			t.Fatalf("write flag fixture %s: %v", name, err)
		}
	}

	code, err := discovery.Load(discovery.Options{
		Root: root, Patterns: nil, BuildTags: nil, CallGraph: false,
		CacheDir:     "",
		BuildFlags:   []string{"-p=8", "-x", "-trimpath", "-trimpath=false", "-buildvcs=auto"},
		IncludeTests: false, IncludeDependencies: false, GOOS: "", GOARCH: "", Offline: true,
		Env: []string{"GOWORK=off", "GOFLAGS="},
	})
	if err != nil {
		t.Fatalf("load normalized flag fixture: %v", err)
	}

	want := []string{"-buildvcs=auto", "-trimpath=false"}
	if !slices.Equal(code.EffectiveBuild.SemanticFlags, want) {
		t.Fatalf("semantic flags = %v; want %v", code.EffectiveBuild.SemanticFlags, want)
	}
}
