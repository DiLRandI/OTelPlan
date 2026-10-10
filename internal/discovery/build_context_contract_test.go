package discovery_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestBuildContextModePrecedence(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"mod", "readonly", "vendor"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			root, directory, files := buildContextFixture(t)

			err := directory.MkdirAll("vendor", 0o700)
			if err != nil {
				t.Fatal(err)
			}

			err = directory.WriteFile("vendor/modules.txt", nil, 0o600)
			if err != nil {
				t.Fatal(err)
			}

			var options discovery.Options

			options.Root, options.Offline = root, true
			options.Env = []string{"GOWORK=off", "CGO_ENABLED=0", "GOFLAGS=-mod=vendor"}
			options.BuildFlags = []string{"-mod=" + mode}

			code, err := discovery.LoadContext(t.Context(), options)
			if err != nil || code.EffectiveBuild.ModuleMode != mode {
				t.Fatalf("explicit module mode changed: code=%+v error=%v", code, err)
			}

			checkBuildContextFiles(t, directory, files)
		})
	}
}

func TestBuildContextTagsAndFingerprint(t *testing.T) {
	t.Parallel()

	root, directory, files := buildContextFixture(t)

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Env = []string{"GOWORK=off", "CGO_ENABLED=0", "GOFLAGS=-tags=ambient -mod=mod -trimpath=false"}
	options.BuildTags = []string{"policy"}
	options.BuildFlags = []string{"-tags=chosen", "-mod=readonly", "-trimpath=true"}
	before := slices.Clone(options.BuildTags)

	code := loadBuildContext(t, options)
	if !slices.Equal(code.EffectiveBuild.BuildTags, []string{"chosen"}) ||
		code.EffectiveBuild.ModuleMode != "readonly" ||
		!slices.Equal(code.EffectiveBuild.SemanticFlags, []string{"-trimpath=true"}) {
		t.Fatalf("effective tags or semantic flags changed: %+v", code.EffectiveBuild)
	}

	if _, exists := code.Symbol("example.com/buildcontext.Chosen"); !exists {
		t.Fatal("explicit tags did not select the requested source")
	}

	first := buildContextDigest(t, code)
	second := buildContextDigest(t, loadBuildContext(t, options))

	if first != second || !slices.Equal(options.BuildTags, before) {
		t.Fatal("identical discovery changed its fingerprint or caller tags")
	}

	options.BuildFlags = []string{"-tags=", "-mod=readonly", "-trimpath=true"}

	cleared := loadBuildContext(t, options)
	if len(cleared.EffectiveBuild.BuildTags) != 0 || buildContextDigest(t, cleared) == first {
		t.Fatal("clearing explicit tags did not change the analyzed build identity")
	}

	if _, exists := cleared.Symbol("example.com/buildcontext.Fallback"); !exists {
		t.Fatal("cleared tags did not select fallback source")
	}

	checkBuildContextFiles(t, directory, files)
}

func TestBuildContextDriverOverride(t *testing.T) {
	t.Parallel()

	root, _, _ := buildContextFixture(t)

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Env = []string{
		"GOWORK=off", "CGO_ENABLED=0", "GOFLAGS=", "GOPACKAGESDRIVER=caller-private-driver", "GOPACKAGESDRIVER=off",
	}
	loadBuildContext(t, options)

	options.Env = append(options.Env, "GOPACKAGESDRIVER=caller-private-driver")

	_, err := discovery.LoadContext(t.Context(), options)
	if err == nil || strings.Contains(err.Error(), "caller-private") ||
		!strings.Contains(err.Error(), "custom GOPACKAGESDRIVER") {
		t.Fatalf("custom driver was not safely rejected: %v", err)
	}
}

func loadBuildContext(t *testing.T, options discovery.Options) *model.CodeModel {
	t.Helper()

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatalf("load build context: %v", err)
	}

	return code
}

func buildContextDigest(t *testing.T, code *model.CodeModel) string {
	t.Helper()

	digest, err := lockfile.GraphDigest(code)
	if err != nil {
		t.Fatalf("fingerprint build context: %v", err)
	}

	return digest
}

func buildContextFixture(t *testing.T) (string, *os.Root, map[string]string) {
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

	files := map[string]string{
		"go.mod":      "module example.com/buildcontext\n\ngo 1.27.0\n",
		"chosen.go":   "//go:build chosen\n\npackage buildcontext\nfunc Chosen() {}\n",
		"fallback.go": "//go:build !chosen\n\npackage buildcontext\nfunc Fallback() {}\n",
	}

	for name, contents := range files {
		err := directory.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	return root, directory, files
}

func checkBuildContextFiles(t *testing.T, directory *os.Root, files map[string]string) {
	t.Helper()

	for name, want := range files {
		got, err := directory.ReadFile(name)
		if err != nil || string(got) != want {
			t.Fatalf("discovery changed %s: %v", name, err)
		}
	}
}
