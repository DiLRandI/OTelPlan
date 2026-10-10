package discovery_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
)

func TestModuleMetadataIsolationKeepsRelativeReplacements(t *testing.T) {
	t.Parallel()

	root, directory, files := buildContextFixture(t)
	files["go.mod"] += "\nrequire example.com/local v0.0.0\nreplace example.com/local => ./local\n"
	files["fallback.go"] = "package buildcontext\nimport _ \"example.com/local\"\nfunc Fallback() {}\n"
	files["local/go.mod"] = "module example.com/local\n\ngo 1.27.0\n"
	files["local/local.go"] = "package local\nfunc Local() {}\n"

	err := directory.Mkdir("local", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	for name, contents := range files {
		err := directory.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Env = []string{"GOWORK=off", "GOFLAGS=-mod=readonly", "CGO_ENABLED=0"}

	code := loadBuildContext(t, options)
	if _, found := code.Symbol("example.com/buildcontext.Fallback"); !found {
		t.Fatal("isolated manifest lost the source package")
	}

	checkBuildContextFiles(t, directory, files)
}

func TestModuleMetadataIsolationFollowsAlternateManifestSymlinks(t *testing.T) {
	t.Parallel()

	root, directory, files := buildContextFixture(t)
	outside := t.TempDir()
	manifest := filepath.Join(outside, "selected.mod")
	checksums := filepath.Join(outside, "selected.sum")

	err := os.WriteFile(manifest, []byte(files["go.mod"]), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(checksums, nil, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Symlink(manifest, filepath.Join(root, "alternate.mod"))
	if err != nil {
		t.Fatalf("create manifest symlink: %v", err)
	}

	err = os.Symlink(checksums, filepath.Join(root, "alternate.sum"))
	if err != nil {
		t.Fatalf("create checksum symlink: %v", err)
	}

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.BuildFlags = []string{"-mod=readonly", "-modfile=alternate.mod"}
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	code := loadBuildContext(t, options)
	if code.EffectiveBuild.ModFile != filepath.Join(root, "alternate.mod") ||
		strings.Contains(code.EffectiveBuild.ModFile, "otelplan-effective") {
		t.Fatalf("temporary or resolved path replaced declared manifest identity: %+v", code.EffectiveBuild)
	}

	checkBuildContextFiles(t, directory, files)
}
