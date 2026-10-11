package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
)

func TestLoadDecodesGoEnvironmentNames(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, contents := range map[string]string{
		"go.mod": "module example.com/environment\n\ngo 1.27\n",
		"app.go": "package environment\nfunc Run() {}\n",
	} {
		err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600)
		if err != nil {
			t.Fatalf("write environment fixture %s: %v", name, err)
		}
	}

	code, err := discovery.Load(discovery.Options{
		Root: root, Patterns: nil, BuildTags: nil, BuildFlags: nil, CallGraph: false,
		CacheDir:     "",
		IncludeTests: false, IncludeDependencies: false, GOOS: "linux", GOARCH: "amd64", Offline: true,
		Env: []string{
			"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0", "CGO_CFLAGS=-DTEST_C=1",
			"CGO_CPPFLAGS=-DTEST_CPP=1", "CGO_CXXFLAGS=-DTEST_CXX=1", "CGO_LDFLAGS=-L/test/library",
			"CGO_FFLAGS=-DTEST_F=1",
		},
	})
	if err != nil {
		t.Fatalf("load environment fixture: %v", err)
	}

	build := code.EffectiveBuild
	for _, setting := range []struct {
		name string
		got  string
		want string
	}{
		{name: "GOOS", got: build.GOOS, want: "linux"},
		{name: "GOARCH", got: build.GOARCH, want: "amd64"},
		{name: "CGO_ENABLED", got: build.CGOEnabled, want: "0"},
		{name: "CGO_CFLAGS", got: build.CGOCFLAGS, want: "-DTEST_C=1"},
		{name: "CGO_CPPFLAGS", got: build.CGOCPPFLAGS, want: "-DTEST_CPP=1"},
		{name: "CGO_CXXFLAGS", got: build.CGOCXXFLAGS, want: "-DTEST_CXX=1"},
		{name: "CGO_LDFLAGS", got: build.CGOLDFLAGS, want: "-L/test/library"},
		{name: "CGO_FFLAGS", got: build.CGOFFLAGS, want: "-DTEST_F=1"},
	} {
		if setting.got != setting.want {
			t.Errorf("%s = %q; want %q", setting.name, setting.got, setting.want)
		}
	}
}
