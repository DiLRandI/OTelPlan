package lockfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/internal/lockfile"
)

func TestEffectiveAnalysisFingerprint(t *testing.T) {
	t.Parallel()

	root, moved := environmentProject(t), environmentProject(t)

	base := analysisDigest(t, root, "-mod=readonly")

	if analysisDigest(t, root, "-mod=readonly") != base || analysisDigest(t, moved, "-mod=readonly") != base {
		t.Fatal("repeat or relocation changed fingerprint")
	}

	if analysisDigest(t, root, "-mod=mod") == base {
		t.Fatal("module mode ignored")
	}

	if analysisDigest(t, root, "-tags=production") == base {
		t.Fatal("ambient tags ignored")
	}

	alternate := analysisDigest(t, root, "-modfile="+filepath.Join(root, "alternate.mod"))
	if analysisDigest(t, moved, "-modfile="+filepath.Join(moved, "alternate.mod")) != alternate {
		t.Fatal("absolute modfile location caused drift")
	}

	contents := "module example.com/app\n\ngo 1.27\n\nexclude example.com/unused v1.0.0\n"

	err := os.WriteFile(filepath.Join(root, "alternate.mod"), []byte(contents), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	if analysisDigest(t, root, "-modfile="+filepath.Join(root, "alternate.mod")) == alternate {
		t.Fatal("alternate manifest change ignored")
	}
}

func TestAlternateManifestSumFingerprint(t *testing.T) {
	t.Parallel()

	code := graphFixture(t)
	code.WorkspaceFile = ""
	code.EffectiveBuild.ModFile = filepath.Join(code.Modules[0].Dir, "alternate.mod")

	original, err := os.ReadFile(filepath.Join(code.Modules[0].Dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}

	directory, err := os.OpenRoot(code.Modules[0].Dir)
	if err != nil {
		t.Fatalf("open alternate manifest fixture directory: %v", err)
	}

	t.Cleanup(func() {
		err := directory.Close()
		if err != nil {
			t.Errorf("close alternate manifest fixture directory: %v", err)
		}
	})

	err = directory.WriteFile("alternate.mod", original, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	before, err := lockfile.GraphDigest(code)
	if err != nil {
		t.Fatal(err)
	}

	contents := "example.com/unused v1.0.0 h1:example\n"

	err = os.WriteFile(filepath.Join(code.Modules[0].Dir, "alternate.sum"), []byte(contents), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	after, err := lockfile.GraphDigest(code)
	if err != nil {
		t.Fatal(err)
	}

	if before == after {
		t.Fatal("alternate.sum change ignored")
	}
}

func TestEffectiveEnvironmentInputs(t *testing.T) {
	t.Parallel()

	code := graphFixture(t)
	code.EffectiveBuild.CGOEnabled = "1"
	code.EffectiveBuild.GOAMD64 = "v1"

	before, err := lockfile.GraphDigest(code)
	if err != nil {
		t.Fatal(err)
	}

	code.EffectiveBuild.GOARM = "5"

	irrelevant, err := lockfile.GraphDigest(code)
	if err != nil || irrelevant != before {
		t.Fatal("inactive architecture affected fingerprint")
	}

	code.EffectiveBuild.GOAMD64 = "v2"

	after, err := lockfile.GraphDigest(code)
	if err != nil || before == after {
		t.Fatal("architecture feature level ignored")
	}

	code.EffectiveBuild.CGOCFLAGS = "-DAPP_LAYOUT=2"

	cgo, err := lockfile.GraphDigest(code)
	if err != nil || cgo == after {
		t.Fatal("cgo configuration ignored")
	}
}

func environmentProject(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	for name, contents := range map[string]string{
		"go.mod":        "module example.com/app\n\ngo 1.27\n",
		"alternate.mod": "module example.com/app\n\ngo 1.27\n",
		"app.go":        "package app\nfunc Run() {}\n",
	} {
		err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600)
		if err != nil {
			t.Fatalf("write environment fixture %s: %v", name, err)
		}
	}

	return root
}

func analysisDigest(t *testing.T, root, flags string) string {
	t.Helper()

	code, err := discovery.Load(discovery.Options{
		Root: root, Offline: true, Env: []string{"GOWORK=off", "GOFLAGS=" + flags},
		Patterns: nil, BuildTags: nil, BuildFlags: nil, CallGraph: false, IncludeTests: false,
		IncludeDependencies: false, GOOS: "", GOARCH: "",
	})
	if err != nil {
		t.Fatalf("load environment fixture: %v", err)
	}

	digest, err := lockfile.GraphDigest(code)
	if err != nil {
		t.Fatalf("fingerprint environment fixture: %v", err)
	}

	return digest
}
