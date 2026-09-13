package discovery

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestDiscoveryBuildFlagOverrides(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{"go.mod": "module example.com/flags\n\ngo 1.27.0\n", "chosen.go": "//go:build chosen\n\npackage flags\nfunc Chosen() {}\n", "fallback.go": "//go:build !chosen\n\npackage flags\nfunc Fallback() {}\n"}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	opts := Options{Root: root, BuildTags: []string{"policy"}, Env: []string{"GOWORK=off", "GOFLAGS=-tags=ambient -trimpath=false -mod=mod"}, BuildFlags: []string{"-tags=chosen", "-trimpath=true", "-mod=readonly"}}
	code, err := LoadContext(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := code.Symbol("example.com/flags.Chosen"); !ok {
		t.Fatal("explicit build tags did not control selected source")
	}
	if !slices.Equal(code.EffectiveBuild.BuildTags, []string{"chosen"}) || code.EffectiveBuild.ModuleMode != "readonly" || !slices.Contains(code.EffectiveBuild.SemanticFlags, "-trimpath=true") {
		t.Fatalf("wrong effective build: %+v", code.EffectiveBuild)
	}
	if !slices.Equal(opts.BuildTags, []string{"policy"}) {
		t.Fatal("caller tags changed")
	}
	opts.BuildFlags = []string{"-tags="}
	code, err = LoadContext(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(code.EffectiveBuild.BuildTags) != 0 {
		t.Fatal("empty explicit tags did not clear defaults")
	}
	if _, ok := code.Symbol("example.com/flags.Fallback"); !ok {
		t.Fatal("empty explicit tags selected wrong source")
	}
	for name, want := range files {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(data) != want {
			t.Fatal("analysis changed source")
		}
	}
}

func TestBuildOverrideTokens(t *testing.T) {
	flags, err := parseGOFLAGS("-mod=mod -tags=ambient", "-mod=readonly", "-tags=two tags", "-modfile=path with spaces.mod")
	if err != nil {
		t.Fatal(err)
	}
	if flags.moduleMode != "readonly" || flags.modFile != "path with spaces.mod" || !slices.Equal(flags.tags, []string{"tags", "two"}) {
		t.Fatalf("token boundaries lost: %+v", flags)
	}
	if _, err := parseGOFLAGS("", "-overlay=private-location"); err == nil {
		t.Fatal("accepted unsupported override")
	}
}
