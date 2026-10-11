package discovery_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
)

func TestBuildOverrideTokens(t *testing.T) {
	t.Parallel()

	root, directory, files := buildOverrideFixture(t)

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Env = []string{"GOWORK=off", "GOFLAGS=-mod=mod -tags=ambient"}
	options.BuildFlags = []string{"-mod=readonly", "-tags=two tags", "-modfile=path with spaces.mod"}
	beforeEnv := slices.Clone(options.Env)
	beforeBuildFlags := slices.Clone(options.BuildFlags)
	beforeBuildTags := slices.Clone(options.BuildTags)

	code := loadBuildContext(t, options)
	if !slices.Equal(code.EffectiveBuild.BuildTags, []string{"tags", "two"}) ||
		code.EffectiveBuild.ModuleMode != "readonly" ||
		code.EffectiveBuild.ModFile != filepath.Join(root, "path with spaces.mod") {
		t.Fatalf("wrong effective build: %+v", code.EffectiveBuild)
	}

	if _, exists := code.Symbol("example.com/alternate.Chosen"); !exists {
		t.Fatal("explicit build tags did not select the alternate module source")
	}

	if _, exists := code.Symbol("example.com/alternate.Fallback"); exists {
		t.Fatal("explicit build tags selected the fallback source")
	}

	if !slices.Equal(options.Env, beforeEnv) || !slices.Equal(options.BuildFlags, beforeBuildFlags) ||
		!slices.Equal(options.BuildTags, beforeBuildTags) {
		t.Fatal("discovery changed caller build inputs")
	}

	checkBuildOverrideFiles(t, directory, files)
}

func TestUnsupportedBuildOverride(t *testing.T) {
	t.Parallel()

	root, directory, files := buildContextFixture(t)

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Env = []string{"GOWORK=off", "GOFLAGS="}
	options.BuildFlags = []string{"-overlay=private-location"}

	_, err := discovery.LoadContext(t.Context(), options)
	if err == nil || strings.Contains(err.Error(), "private-location") ||
		!strings.Contains(err.Error(), "unsupported GOFLAGS option -overlay") {
		t.Fatalf("unsupported overlay was not safely rejected: %v", err)
	}

	checkBuildOverrideFiles(t, directory, files)
}

func buildOverrideFixture(t *testing.T) (string, *os.Root, map[string]string) {
	t.Helper()

	root, directory, files := buildContextFixture(t)
	alternate := "module example.com/alternate\n\ngo 1.27.0\n"
	files["path with spaces.mod"] = alternate
	files["chosen.go"] = "//go:build tags && two\n\npackage buildcontext\nfunc Chosen() {}\n"
	files["fallback.go"] = "//go:build !(tags && two)\n\npackage buildcontext\nfunc Fallback() {}\n"

	writeOverrideFile(t, directory, "path with spaces.mod", files["path with spaces.mod"])
	writeOverrideFile(t, directory, "chosen.go", files["chosen.go"])
	writeOverrideFile(t, directory, "fallback.go", files["fallback.go"])

	return root, directory, files
}

func writeOverrideFile(t *testing.T, directory *os.Root, name, contents string) {
	t.Helper()

	err := directory.WriteFile(name, []byte(contents), 0o600)
	if err != nil {
		t.Fatal(err)
	}
}

func checkBuildOverrideFiles(t *testing.T, directory *os.Root, files map[string]string) {
	t.Helper()

	entries, err := fs.ReadDir(directory.FS(), ".")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != len(files) {
		t.Fatalf("discovery changed project file set: got %d entries, want %d", len(entries), len(files))
	}

	for _, entry := range entries {
		if _, exists := files[entry.Name()]; !exists {
			t.Fatalf("discovery added project file %s", entry.Name())
		}
	}

	checkBuildContextFiles(t, directory, files)
}
