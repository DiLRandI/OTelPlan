package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

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

func readCacheFixture(t *testing.T, path string) map[string][]byte {
	t.Helper()

	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		t.Fatal(err)
	}

	files := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			files[entry.Name()] = nil

			continue
		}

		contents, err := root.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}

		files[entry.Name()] = contents
	}

	return files
}

func assertCacheFixtureEqual(t *testing.T, want, got map[string][]byte) {
	t.Helper()

	if !reflect.DeepEqual(want, got) {
		t.Fatalf("cache files changed: want %v, got %v", want, got)
	}
}

func TestAnalysisCacheReadOnlyValidHitRestoresEnvironment(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\nfunc Original() {}\n"})
	opts.Env = append(opts.Env, "CGO_CFLAGS=read-only-value")
	loadCacheTest(t, opts)
	want := readCacheFixture(t, opts.CacheDir)

	opts.CacheReadOnly = true

	code, hit := loadCacheTest(t, opts)
	if !hit || code.EffectiveBuild.CGOCFLAGS != "read-only-value" {
		t.Fatal("read-only cache did not return a hit with the current effective environment")
	}

	assertCacheFixtureEqual(t, want, readCacheFixture(t, opts.CacheDir))
}

func TestAnalysisCacheReadOnlyMissingDirectoryDoesNotCreate(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\nfunc Original() {}\n"})
	opts.CacheDir = filepath.Join(opts.CacheDir, "nested", "missing")
	opts.CacheReadOnly = true

	_, hit := loadCacheTest(t, opts)
	if hit {
		t.Fatal("read-only cache unexpectedly hit a missing directory")
	}

	_, err := os.Stat(opts.CacheDir)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only cache created missing directory: %v", err)
	}

	nested := filepath.Dir(opts.CacheDir)

	_, err = os.Stat(nested)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only cache created missing parent directory: %v", err)
	}
}

func TestAnalysisCacheReadOnlySourceInvalidationPreservesEntry(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\nfunc Original() {}\n"})
	loadCacheTest(t, opts)
	want := readCacheFixture(t, opts.CacheDir)

	writeCacheFixture(t, opts.Root, "app.go", "package shop\nfunc Changed() {}\n")
	opts.CacheReadOnly = true

	code, hit := loadCacheTest(t, opts)
	if hit || len(code.Symbols) != 1 || code.Symbols[0].Name != "Changed" {
		t.Fatal("read-only source invalidation did not produce fresh analysis")
	}

	assertCacheFixtureEqual(t, want, readCacheFixture(t, opts.CacheDir))
}

func TestAnalysisCacheReadOnlyCorruptEntryPreservesBytes(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\nfunc Original() {}\n"})
	loadCacheTest(t, opts)

	entries := readCacheFixture(t, opts.CacheDir)
	if len(entries) != 1 {
		t.Fatalf("cache entries=%d; want one", len(entries))
	}

	for name, contents := range entries {
		if contents == nil {
			continue
		}

		writeCacheFixture(t, opts.CacheDir, name, "{broken")
	}

	want := readCacheFixture(t, opts.CacheDir)

	opts.CacheReadOnly = true

	code, hit := loadCacheTest(t, opts)
	if hit || len(code.Symbols) != 1 || code.Symbols[0].Name != "Original" {
		t.Fatal("read-only corrupt cache did not produce fresh analysis")
	}

	assertCacheFixtureEqual(t, want, readCacheFixture(t, opts.CacheDir))
}

func TestAnalysisCacheReadOnlyEmptyCacheStaysEmpty(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\nfunc Original() {}\n"})
	opts.CacheReadOnly = true

	_, hit := loadCacheTest(t, opts)
	if hit {
		t.Fatal("read-only empty cache unexpectedly hit")
	}

	if files := readCacheFixture(t, opts.CacheDir); len(files) != 0 {
		t.Fatalf("read-only empty cache was populated: %v", files)
	}
}

func TestAnalysisCacheStorageCause(t *testing.T) {
	t.Parallel()

	opts := cacheTestOptions(t, map[string]string{"app.go": "package shop\n"})
	cacheFile := filepath.Join(opts.Root, "cache-file")
	writeCacheFixture(t, opts.Root, "cache-file", "cache path is a file")
	opts.CacheDir = cacheFile

	_, _, err := loadContextResult(t.Context(), opts)
	if !errors.Is(err, ErrCacheStorage) {
		t.Fatalf("storage error=%v; want errors.Is(..., ErrCacheStorage)", err)
	}

	if !errors.Is(err, syscall.ENOTDIR) {
		t.Fatalf("storage error=%v; want underlying ENOTDIR cause", err)
	}

	opts.CacheReadOnly = true

	_, _, err = loadContextResult(t.Context(), opts)
	if !errors.Is(err, ErrCacheStorage) {
		t.Fatalf("read-only storage error=%v; want errors.Is(..., ErrCacheStorage)", err)
	}

	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || pathErr.Err == nil {
		t.Fatalf("read-only storage error=%v; want retained filesystem cause", err)
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

func TestAnalysisCacheRetentionCountBound(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	root := openRetentionRoot(t, rootPath)
	keys := []string{retentionKey('a'), retentionKey('b'), retentionKey('c'), retentionKey('d')}

	for index, key := range keys {
		writeRetentionEntry(t, root, key, newRetentionModel())
		setRetentionTime(t, root, key, time.Unix(int64(index), 0))
	}

	want := map[string][]byte{keys[2] + ".json": nil, keys[3] + ".json": nil}

	err := pruneAnalysisCache(t.Context(), root, keys[3], 2, 1<<20)
	if err != nil {
		t.Fatal(err)
	}

	assertRetentionNames(t, root, want)
}

func TestAnalysisCacheRetentionByteBound(t *testing.T) {
	t.Parallel()

	root := openRetentionRoot(t, t.TempDir())
	keys := []string{retentionKey('a'), retentionKey('b'), retentionKey('c')}

	for index, key := range keys {
		code := newRetentionModel()
		code.Symbols = make([]model.Symbol, index*4)
		writeRetentionEntry(t, root, key, code)
		setRetentionTime(t, root, key, time.Unix(int64(index), 0))
	}

	keepSize := retentionFileSize(t, root, keys[2])
	middleSize := retentionFileSize(t, root, keys[1])
	requireRetentionPrune(t, root, keys[2], 10, keepSize+middleSize)

	assertRetentionNames(t, root, map[string][]byte{keys[1] + ".json": nil, keys[2] + ".json": nil})
}

func TestAnalysisCacheRetentionKeepAndTieBreak(t *testing.T) {
	t.Parallel()

	root := openRetentionRoot(t, t.TempDir())
	keys := []string{retentionKey('a'), retentionKey('b'), retentionKey('c'), retentionKey('f')}

	for _, key := range keys {
		writeRetentionEntry(t, root, key, newRetentionModel())

		stamp := time.Unix(42, 0)
		if key == keys[3] {
			stamp = time.Unix(1, 0)
		}

		setRetentionTime(t, root, key, stamp)
	}

	err := pruneAnalysisCache(t.Context(), root, keys[3], 2, 1<<20)
	if err != nil {
		t.Fatal(err)
	}

	assertRetentionNames(t, root, map[string][]byte{keys[2] + ".json": nil, keys[3] + ".json": nil})
}

func TestAnalysisCacheRetentionPreservesUnrecognizedEntries(t *testing.T) {
	t.Parallel()

	root := openRetentionRoot(t, t.TempDir())
	valid := retentionKey('a')
	keep := retentionKey('b')

	writeRetentionEntry(t, root, valid, newRetentionModel())
	writeRetentionEntry(t, root, keep, newRetentionModel())
	setRetentionTime(t, root, valid, time.Unix(1, 0))
	setRetentionTime(t, root, keep, time.Unix(2, 0))

	preserved := map[string][]byte{
		"not-hex.json": []byte("ordinary"),
		strings.ToUpper(retentionKey('c')) + ".json": []byte("uppercase"),
		retentionKey('d') + ".json":                  []byte("{broken"),
		retentionKey('e') + ".json":                  retentionEntryVariant(t, 1, "wrong"),
		retentionKey('f') + ".json":                  retentionEntryBadDigest(t, retentionKey('f')),
		retentionKey('1') + ".json":                  retentionEntryVariant(t, 2, retentionKey('1')),
		"ordinary":                                   []byte("file"),
	}
	badModel := new(model.CodeModel)
	badModel.GoVersion = "go1.27.2"
	badModelName := retentionKey('3') + ".json"

	preserved[badModelName] = retentionEntryBytes(t, retentionKey('3'), badModel)
	for name, contents := range preserved {
		writeRetentionFile(t, root, name, contents)
	}

	directory := retentionKey('2') + ".json"

	err := root.Mkdir(directory, 0o700)
	if err != nil {
		t.Fatal(err)
	}

	preserved[directory] = nil
	want := retentionSnapshot(t, root)

	err = pruneAnalysisCache(t.Context(), root, keep, 1, 1<<20)
	if err != nil {
		t.Fatal(err)
	}

	got := retentionSnapshot(t, root)

	for name, contents := range want {
		if name == valid+".json" {
			continue
		}

		gotContents, ok := got[name]
		if !ok || !bytes.Equal(contents, gotContents) {
			t.Fatalf("unrecognized entry %s changed", name)
		}
	}

	if _, ok := got[valid+".json"]; ok {
		t.Fatalf("oldest recognized entry %s was retained", valid)
	}
}

func TestAnalysisCacheRetentionCancellation(t *testing.T) {
	t.Parallel()

	root := openRetentionRoot(t, t.TempDir())
	key := retentionKey('a')
	writeRetentionEntry(t, root, key, newRetentionModel())
	before := retentionSnapshot(t, root)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := pruneAnalysisCache(ctx, root, "", 1, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("retention error=%v; want context.Canceled", err)
	}

	if !reflect.DeepEqual(before, retentionSnapshot(t, root)) {
		t.Fatal("canceled retention mutated cache files")
	}
}

func TestAnalysisCacheRetentionWritablePublication(t *testing.T) {
	t.Parallel()

	root := openRetentionRoot(t, t.TempDir())

	const publications = 65

	lastKey := ""
	for index := range publications {
		lastKey = fmt.Sprintf("%064x", index)

		err := writeAnalysisCache(t.Context(), root, lastKey, newRetentionModel())
		if err != nil {
			t.Fatal(err)
		}
	}

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) > maximumAnalysisCacheEntries {
		t.Fatalf("cache entries=%d; want at most %d", len(entries), maximumAnalysisCacheEntries)
	}

	var opts Options

	opts.Root, opts.goVersion = "module", "go1.27.2"

	lookup, err := readAnalysisCache(root, lastKey, opts)
	if err != nil {
		t.Fatal(err)
	}

	if lookup.code == nil || lookup.code.ModuleRoot != "module" {
		t.Fatal("current publication was pruned or invalid")
	}
}

func TestAnalysisCacheRetentionConcurrentPublication(t *testing.T) {
	t.Parallel()

	root := openRetentionRoot(t, t.TempDir())

	const publications = 66

	var workers sync.WaitGroup

	errorsCh := make(chan error, publications)
	workers.Add(publications)

	for index := range publications {
		go func(index int) {
			defer workers.Done()

			key := fmt.Sprintf("%064x", index+publications)

			err := writeAnalysisCache(t.Context(), root, key, newRetentionModel())
			if err != nil {
				errorsCh <- err
			}
		}(index)
	}

	workers.Wait()

	close(errorsCh)

	for err := range errorsCh {
		t.Fatal(err)
	}

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) > maximumAnalysisCacheEntries {
		t.Fatalf("concurrent cache entries=%d; want at most %d", len(entries), maximumAnalysisCacheEntries)
	}
}

func TestAnalysisCacheRetentionCanceledPublication(t *testing.T) {
	t.Parallel()

	root := openRetentionRoot(t, t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := writeAnalysisCache(ctx, root, fmt.Sprintf("%064x", 1), newRetentionModel())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("write error=%v; want context.Canceled", err)
	}

	entries, readErr := fs.ReadDir(root.FS(), ".")
	if readErr != nil {
		t.Fatal(readErr)
	}

	if len(entries) != 0 {
		t.Fatalf("canceled publication created %d entries", len(entries))
	}
}

func openRetentionRoot(t *testing.T, path string) *os.Root {
	t.Helper()

	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	return root
}

func retentionKey(letter byte) string {
	return strings.Repeat(string(letter), 64)
}

func newRetentionModel() *model.CodeModel {
	code := new(model.CodeModel)
	code.ModuleRoot, code.GoVersion = "module", "go1.27.2"

	return code
}

func writeRetentionEntry(t *testing.T, root *os.Root, key string, code *model.CodeModel) {
	t.Helper()

	contents := retentionEntryBytes(t, key, code)
	writeRetentionFile(t, root, key+".json", contents)
}

func retentionEntryBytes(t *testing.T, key string, code *model.CodeModel) []byte {
	t.Helper()

	payload := marshalRetentionJSON(t, code)

	entry := analysisCacheEntry{Version: analysisCacheVersion, Key: key, Digest: cacheDigest(payload), Model: payload}
	contents := marshalRetentionJSON(t, entry)

	return contents
}

func retentionEntryVariant(t *testing.T, version int, entryKey string) []byte {
	t.Helper()

	payload := marshalRetentionJSON(t, newRetentionModel())
	entry := analysisCacheEntry{Version: version, Key: entryKey, Digest: cacheDigest(payload), Model: payload}

	return marshalRetentionJSON(t, entry)
}

func retentionEntryBadDigest(t *testing.T, key string) []byte {
	t.Helper()

	payload := marshalRetentionJSON(t, newRetentionModel())
	entry := analysisCacheEntry{Version: analysisCacheVersion, Key: key, Digest: "bad", Model: payload}

	return marshalRetentionJSON(t, entry)
}

func marshalRetentionJSON(t *testing.T, value any) []byte {
	t.Helper()

	contents, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	return contents
}

func requireRetentionPrune(t *testing.T, root *os.Root, keep string, maximumEntries int, maximumBytes int64) {
	t.Helper()

	err := pruneAnalysisCache(t.Context(), root, keep, maximumEntries, maximumBytes)
	if err != nil {
		t.Fatal(err)
	}
}

func writeRetentionFile(t *testing.T, root *os.Root, name string, contents []byte) {
	t.Helper()

	err := root.WriteFile(name, contents, cacheFileMode)
	if err != nil {
		t.Fatal(err)
	}
}

func setRetentionTime(t *testing.T, root *os.Root, key string, stamp time.Time) {
	t.Helper()

	err := root.Chtimes(key+".json", stamp, stamp)
	if err != nil {
		t.Fatal(err)
	}
}

func retentionFileSize(t *testing.T, root *os.Root, key string) int64 {
	t.Helper()

	info, err := root.Stat(key + ".json")
	if err != nil {
		t.Fatal(err)
	}

	return info.Size()
}

func retentionSnapshot(t *testing.T, root *os.Root) map[string][]byte {
	t.Helper()

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		t.Fatal(err)
	}

	snapshot := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			snapshot[entry.Name()] = nil

			continue
		}

		contents, err := root.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}

		snapshot[entry.Name()] = contents
	}

	return snapshot
}

func assertRetentionNames(t *testing.T, root *os.Root, want map[string][]byte) {
	t.Helper()

	got := retentionSnapshot(t, root)
	if len(got) != len(want) {
		t.Fatalf("retained entry count=%d; want %d", len(got), len(want))
	}

	for name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("retained entries=%v; want %v", got, want)
		}
	}
}
