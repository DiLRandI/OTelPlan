package lockfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
)

func TestEffectiveAnalysisFingerprint(t *testing.T) {
	makeProject := func() string {
		root := t.TempDir()
		for name, contents := range map[string]string{
			"go.mod":        "module example.com/app\n\ngo 1.27\n",
			"alternate.mod": "module example.com/app\n\ngo 1.27\n",
			"app.go":        "package app\nfunc Run() {}\n",
		} {
			if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return root
	}
	digest := func(root, flags string) string {
		code, err := discovery.Load(discovery.Options{Root: root, Offline: true, Env: []string{"GOWORK=off", "GOFLAGS=" + flags}})
		if err != nil {
			t.Fatal(err)
		}
		got, err := GraphDigest(code)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	root, moved := makeProject(), makeProject()
	base := digest(root, "-mod=readonly")
	if digest(root, "-mod=readonly") != base || digest(moved, "-mod=readonly") != base {
		t.Fatal("repeat or relocation changed fingerprint")
	}
	if digest(root, "-mod=mod") == base {
		t.Fatal("module mode ignored")
	}
	if digest(root, "-tags=production") == base {
		t.Fatal("ambient tags ignored")
	}
	alternate := digest(root, "-modfile="+filepath.Join(root, "alternate.mod"))
	if digest(moved, "-modfile="+filepath.Join(moved, "alternate.mod")) != alternate {
		t.Fatal("absolute modfile location caused drift")
	}
	if err := os.WriteFile(filepath.Join(root, "alternate.mod"), []byte("module example.com/app\n\ngo 1.27\n\nexclude example.com/unused v1.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if digest(root, "-modfile="+filepath.Join(root, "alternate.mod")) == alternate {
		t.Fatal("alternate manifest change ignored")
	}
}

func TestAlternateManifestSumFingerprint(t *testing.T) {
	code := graphFixture(t)
	code.WorkspaceFile = ""
	code.EffectiveBuild.ModFile = filepath.Join(code.Modules[0].Dir, "alternate.mod")
	original, err := os.ReadFile(filepath.Join(code.Modules[0].Dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(code.EffectiveBuild.ModFile, original, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := GraphDigest(code)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(companionSum(code.EffectiveBuild.ModFile), []byte("example.com/unused v1.0.0 h1:example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := GraphDigest(code)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("alternate.sum change ignored")
	}
}

func TestEffectiveEnvironmentInputs(t *testing.T) {
	code := graphFixture(t)
	code.EffectiveBuild.CGOEnabled = "1"
	code.EffectiveBuild.GOAMD64 = "v1"
	before, err := GraphDigest(code)
	if err != nil {
		t.Fatal(err)
	}
	code.EffectiveBuild.GOARM = "5"
	irrelevant, err := GraphDigest(code)
	if err != nil || irrelevant != before {
		t.Fatal("inactive architecture affected fingerprint")
	}
	code.EffectiveBuild.GOAMD64 = "v2"
	after, err := GraphDigest(code)
	if err != nil || before == after {
		t.Fatal("architecture feature level ignored")
	}
	code.EffectiveBuild.CGOCFLAGS = "-DAPP_LAYOUT=2"
	cgo, err := GraphDigest(code)
	if err != nil || cgo == after {
		t.Fatal("cgo configuration ignored")
	}
}
