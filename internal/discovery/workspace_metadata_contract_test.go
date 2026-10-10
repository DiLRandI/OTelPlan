package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
)

func TestWorkspaceMetadataPreservesLocalReplacements(t *testing.T) {
	t.Parallel()

	root, directory, files := workspaceMetadataFixture(t)

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Env = []string{"GOWORK=" + filepath.Join(root, "go.work"), "GOFLAGS=-mod=readonly", "CGO_ENABLED=0"}

	code := loadBuildContext(t, options)
	if _, found := code.Symbol("example.com/app.Run"); !found {
		t.Fatal("isolated workspace lost its selected module")
	}

	if code.WorkspaceFile != filepath.Join(root, "go.work") || !code.EffectiveBuild.Workspace {
		t.Fatalf("isolated path replaced declared workspace identity: %+v", code.EffectiveBuild)
	}

	first := buildContextDigest(t, code)
	second := buildContextDigest(t, loadBuildContext(t, options))

	if first != second {
		t.Fatal("workspace isolation changed repeated fingerprints")
	}

	checkBuildContextFiles(t, directory, files)
}

func TestWorkspaceMetadataAcceptsSymlinkedChecksums(t *testing.T) {
	t.Parallel()

	root, directory, files := workspaceMetadataFixture(t)
	outside := t.TempDir()
	checksums := filepath.Join(outside, "workspace.sum")

	err := os.WriteFile(checksums, nil, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Symlink(checksums, filepath.Join(root, "go.work.sum"))
	if err != nil {
		t.Fatalf("create workspace checksum symlink: %v", err)
	}

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Env = []string{"GOWORK=" + filepath.Join(root, "go.work"), "GOFLAGS=-mod=readonly", "CGO_ENABLED=0"}

	loadBuildContext(t, options)

	checkBuildContextFiles(t, directory, files)
}

func workspaceMetadataFixture(t *testing.T) (string, *os.Root, map[string]string) {
	t.Helper()

	root := t.TempDir()

	directory, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := directory.Close()
		if err != nil {
			t.Error(err)
		}
	})

	for _, name := range []string{"app", "local"} {
		err := directory.Mkdir(name, 0o700)
		if err != nil {
			t.Fatal(err)
		}
	}

	files := map[string]string{
		"go.work":        "go 1.27.0\nuse ./app\nreplace example.com/local => ./local\n",
		"app/go.mod":     "module example.com/app\n\ngo 1.27.0\nrequire example.com/local v0.0.0\n",
		"app/app.go":     "package app\nimport _ \"example.com/local\"\nfunc Run() {}\n",
		"local/go.mod":   "module example.com/local\n\ngo 1.27.0\n",
		"local/local.go": "package local\nfunc Local() {}\n",
	}

	for name, contents := range files {
		err := directory.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	return root, directory, files
}
