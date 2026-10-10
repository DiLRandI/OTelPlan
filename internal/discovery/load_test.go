package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/tools/go/packages"
)

func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()

	files["go.mod"] = "module example.com/shop\n\ngo 1.27\n"

	for name, content := range files {
		path := filepath.Join(root, name)

		err := os.MkdirAll(filepath.Dir(path), 0o700)
		if err != nil {
			t.Fatal(err)
		}

		err = os.WriteFile(path, []byte(content), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func TestPackageAnalysisErrorsRemainSorted(t *testing.T) {
	t.Parallel()

	dependency := new(packages.Package)
	dependency.ID = "example.com/dependency"
	dependency.Errors = []packages.Error{
		{Pos: "z.go:2:1", Msg: "last failure", Kind: packages.TypeError},
		{Pos: "a.go:1:1", Msg: "first failure", Kind: packages.ParseError},
	}
	root := new(packages.Package)
	root.ID = "example.com/app"
	root.Imports = map[string]*packages.Package{"example.com/dependency": dependency}
	root.Errors = []packages.Error{{Pos: "m.go:3:1", Msg: "middle failure", Kind: packages.TypeError}}

	err := reportErrors([]*packages.Package{root, dependency})
	want := "package analysis failed: a.go:1:1: first failure; m.go:3:1: middle failure; z.go:2:1: last failure"

	if err == nil || err.Error() != want {
		t.Fatalf("analysis errors=%v; want %q", err, want)
	}

	if !errors.Is(err, errPackageAnalysis) {
		t.Fatalf("analysis errors lost their stable cause: %v", err)
	}
}

func cacheTestOptions(t *testing.T, files map[string]string) Options {
	t.Helper()

	opts := new(Options)
	opts.Root, opts.CacheDir, opts.Offline = fixture(t, files), t.TempDir(), true
	opts.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	return *opts
}

func loadCacheTest(t *testing.T, opts Options) (*model.CodeModel, bool) {
	t.Helper()

	code, hit, err := loadContextResult(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}

	return code, hit
}

func writeCacheFixture(t *testing.T, root, name, content string) {
	t.Helper()

	directory, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = directory.Close() }()

	err = directory.WriteFile(name, []byte(content), cacheFileMode)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAnalysisCacheHitAndSourceInvalidation(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\nfunc Original() {}\n"})

	first, hit := loadCacheTest(t, opts)
	if hit || len(first.Symbols) != 1 {
		t.Fatal("first analysis was cached or incomplete")
	}

	first.Symbols[0].Name = "caller mutation"

	second, hit := loadCacheTest(t, opts)
	if !hit || second.Symbols[0].Name != "Original" {
		t.Fatal("cache did not restore fresh metadata")
	}

	writeCacheFixture(t, opts.Root, "app.go", "package shop\nfunc Changed() {}\n")

	changed, hit := loadCacheTest(t, opts)
	if hit || changed.Symbols[0].Name != "Changed" {
		t.Fatal("source content change did not invalidate semantic metadata")
	}
}

func TestAnalysisCacheNewAndDeletedSources(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\nfunc Original() {}\n"})
	loadCacheTest(t, opts)
	writeCacheFixture(t, opts.Root, "new.go", "package shop\nfunc Added() {}\n")

	added, hit := loadCacheTest(t, opts)
	if hit || len(added.Symbols) != 2 {
		t.Fatal("new source did not invalidate semantic metadata")
	}

	directory, err := os.OpenRoot(opts.Root)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = directory.Close() }()

	err = directory.Remove("app.go")
	if err != nil {
		t.Fatal(err)
	}

	removed, hit := loadCacheTest(t, opts)
	if hit || len(removed.Symbols) != 1 || removed.Symbols[0].Name != "Added" {
		t.Fatal("deleted source did not invalidate semantic metadata")
	}
}

func TestAnalysisCacheDoesNotPersistBuildSecrets(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\nfunc Original() {}\n"})
	opts.Env = append(opts.Env, "CGO_CFLAGS=private-cache-value")
	loadCacheTest(t, opts)

	second, hit := loadCacheTest(t, opts)
	if !hit || second.EffectiveBuild.CGOCFLAGS != "private-cache-value" {
		t.Fatal("cache did not restore current effective environment")
	}

	cache, err := os.OpenRoot(opts.CacheDir)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = cache.Close() }()

	entries, err := fs.ReadDir(cache.FS(), ".")
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		contents, err := cache.ReadFile(entry.Name())
		if err != nil || bytes.Contains(contents, []byte("private-cache-value")) {
			t.Fatal("cache persisted sensitive effective environment values")
		}
	}
}

func TestAnalysisCacheCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\n"})

	_, _, err := loadContextResult(ctx, opts)
	if err == nil {
		t.Fatal("canceled analysis returned cached metadata")
	}
}

func TestAnalysisCacheOptionsInvalidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(*Options)
	}{
		{name: "tags", change: func(o *Options) { o.BuildTags = []string{"custom"} }},
		{name: "module mode", change: func(o *Options) { o.BuildFlags = []string{"-mod=mod"} }},
		{name: "platform", change: func(o *Options) { o.GOARCH = "386" }},
		{name: "call graph", change: func(o *Options) { o.CallGraph = true }},
		{name: "tests", change: func(o *Options) { o.IncludeTests = true }},
		{name: "dependencies", change: func(o *Options) { o.IncludeDependencies = true }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\nfunc Original() {}\n"})
			loadCacheTest(t, opts)
			testCase.change(&opts)

			_, hit := loadCacheTest(t, opts)
			if hit {
				t.Fatal("changed analysis option reused old semantic metadata")
			}

			_, hit = loadCacheTest(t, opts)
			if !hit {
				t.Fatal("identical changed analysis was not cached")
			}
		})
	}
}

func TestAnalysisCacheFailuresAreNotCached(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\nfunc Original() {}\n"})
	loadCacheTest(t, opts)
	writeCacheFixture(t, opts.Root, "app.go", "package shop\nfunc Broken( {}\n")

	_, _, err := loadContextResult(t.Context(), opts)
	if err == nil {
		t.Fatal("invalid source returned stale cached metadata")
	}

	writeCacheFixture(t, opts.Root, "app.go", "package shop\nfunc Fixed() {}\n")

	code, hit := loadCacheTest(t, opts)
	if hit || code.Symbols[0].Name != "Fixed" {
		t.Fatal("failed analysis polluted the cache")
	}
}

func TestAnalysisCacheConcurrentPublication(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\nfunc Original() {}\n"})

	const writers = 4

	var workers sync.WaitGroup

	for range writers {
		workers.Go(func() {
			code, _, err := loadContextResult(t.Context(), opts)
			if err != nil {
				t.Error(err)
			} else if len(code.Symbols) != 1 {
				t.Error("concurrent analysis was incomplete")
			}
		})
	}

	workers.Wait()

	_, hit := loadCacheTest(t, opts)
	if !hit {
		t.Fatal("concurrent publication did not leave a valid entry")
	}
}

func TestAnalysisCacheCorruptionTriggersRefresh(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\nfunc Original() {}\n"})
	first, _ := loadCacheTest(t, opts)

	cache, err := os.OpenRoot(opts.CacheDir)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = cache.Close() }()

	entries, err := fs.ReadDir(cache.FS(), ".")
	if err != nil || len(entries) != 1 {
		t.Fatal("analysis did not publish one cache entry")
	}

	err = cache.WriteFile(entries[0].Name(), []byte("{broken"), cacheFileMode)
	if err != nil {
		t.Fatal(err)
	}

	refreshed, hit := loadCacheTest(t, opts)
	if hit || !reflect.DeepEqual(first, refreshed) {
		t.Fatal("corrupt cache did not produce equivalent fresh analysis")
	}
}

func TestAnalysisCacheEntryValidation(t *testing.T) {
	t.Parallel()

	opts := new(Options)
	opts.Root, opts.goVersion = "project", "go-test"
	code := new(model.CodeModel)
	code.ModuleRoot, code.GoVersion = opts.Root, opts.goVersion

	payload, err := json.Marshal(code)
	if err != nil {
		t.Fatal(err)
	}

	entry := analysisCacheEntry{Version: analysisCacheVersion, Key: "key", Digest: cacheDigest(payload), Model: payload}

	tests := []struct {
		name   string
		change func(*analysisCacheEntry)
	}{
		{name: "version", change: func(e *analysisCacheEntry) { e.Version++ }},
		{name: "key", change: func(e *analysisCacheEntry) { e.Key = "other" }},
		{name: "digest", change: func(e *analysisCacheEntry) { e.Digest = "corrupt" }},
		{name: "model", change: func(e *analysisCacheEntry) { e.Model = []byte("null"); e.Digest = cacheDigest(e.Model) }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			invalid := entry
			testCase.change(&invalid)

			encoded, err := json.Marshal(invalid)
			if err != nil {
				t.Fatal(err)
			}

			if decodeAnalysisCache(encoded, "key", *opts).code != nil {
				t.Fatal("invalid cache entry accepted")
			}
		})
	}
}

func TestAnalysisCacheStorageErrors(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\n"})
	opts.CacheDir = filepath.Join(opts.Root, "app.go")

	_, _, err := loadContextResult(t.Context(), opts)
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatal("explicit invalid cache storage was silently ignored")
	}
}

func TestAnalysisCacheLocalReplacementInvalidation(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{
		"app.go":              "package shop\nimport \"example.com/dependency\"\nfunc Use(value dependency.Value) {}\n",
		"dependency/go.mod":   "module example.com/dependency\n\ngo 1.27\n",
		"dependency/value.go": "package dependency\ntype Value struct { Count int }\n",
	})
	writeCacheFixture(t, opts.Root, "go.mod", "module example.com/shop\n\ngo 1.27\n"+
		"require example.com/dependency v0.0.0\nreplace example.com/dependency => ./dependency\n")
	first, _ := loadCacheTest(t, opts)

	_, hit := loadCacheTest(t, opts)
	if !hit {
		t.Fatal("local replacement analysis did not cache")
	}

	writeCacheFixture(t, opts.Root, "dependency/value.go", "package dependency\ntype Value struct { Count string }\n")

	changed, hit := loadCacheTest(t, opts)
	if hit || reflect.DeepEqual(first, changed) {
		t.Fatal("changed replacement type reused old semantic metadata")
	}
}

func TestAnalysisCacheAlternateManifestInvalidation(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\nfunc Original() {}\n"})
	writeCacheFixture(t, opts.Root, "alternate.mod", "module example.com/shop\n\ngo 1.27\n")
	opts.BuildFlags = []string{"-modfile=alternate.mod"}
	loadCacheTest(t, opts)

	_, hit := loadCacheTest(t, opts)
	if !hit {
		t.Fatal("alternate manifest did not cache")
	}

	writeCacheFixture(t, opts.Root, "alternate.mod", "module example.com/shop\n\ngo 1.27\n// changed input\n")

	_, hit = loadCacheTest(t, opts)
	if hit {
		t.Fatal("changed alternate manifest reused old cache")
	}
}

func TestAnalysisCacheWorkspaceInvalidation(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{
		"go.work":    "go 1.27\nuse ./app\n",
		"app/go.mod": "module example.com/application\n\ngo 1.27\n",
		"app/app.go": "package application\nfunc Original() {}\n",
	})
	opts.Patterns = []string{"./app/..."}
	opts.Env = append(opts.Env, "GOWORK="+filepath.Join(opts.Root, "go.work"))
	loadCacheTest(t, opts)

	_, hit := loadCacheTest(t, opts)
	if !hit {
		t.Fatal("workspace analysis did not cache")
	}

	writeCacheFixture(t, opts.Root, "go.work", "go 1.27\nuse ./app\n// changed input\n")

	_, hit = loadCacheTest(t, opts)
	if hit {
		t.Fatal("changed workspace reused old cache")
	}
}

func TestAnalysisCacheOversizedEntryIsIgnored(t *testing.T) {
	t.Parallel()

	root, err := openAnalysisCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	file, err := root.Create("key.json")
	if err != nil {
		t.Fatal(err)
	}

	err = file.Truncate(maximumCacheEntryBytes + 1)

	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatal("could not prepare oversized cache fixture")
	}

	opts := new(Options)

	lookup, err := readAnalysisCache(root, "key", *opts)
	if err != nil || lookup.code != nil {
		t.Fatal("oversized cache was not treated as a miss")
	}
}

func TestAnalysisCacheMatchesUncachedAnalysis(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\nfunc Original() {}\n"})
	freshOptions := opts
	freshOptions.CacheDir = ""

	fresh, hit := loadCacheTest(t, freshOptions)
	if hit {
		t.Fatal("cache used without an explicit directory")
	}

	loadCacheTest(t, opts)

	cached, hit := loadCacheTest(t, opts)
	if !hit || !reflect.DeepEqual(fresh, cached) {
		t.Fatal("cached metadata differs from ordinary analysis")
	}
}

func TestAnalysisCacheModuleLanguageVersionInvalidates(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\ntype Alias[T any] = []T\n"})
	loadCacheTest(t, opts)
	writeCacheFixture(t, opts.Root, "go.mod", "module example.com/shop\n\ngo 1.23\n")

	_, hit := loadCacheTest(t, opts)
	if hit {
		t.Fatal("changed module language version reused previous semantic metadata")
	}
}
