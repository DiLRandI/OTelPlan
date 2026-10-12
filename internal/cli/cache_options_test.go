package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestCLIAnalysisCacheUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "version", args: []string{"version", "--cache-dir", "private-cache"},
			want: "--cache-dir requires a project analysis command"},
		{name: "help", args: []string{"help", "--cache-dir", "private-cache"},
			want: "--cache-dir requires a project analysis command"},
		{name: "unknown command", args: []string{"unknown", "--cache-dir", "private-cache"},
			want: "--cache-dir requires a project analysis command"},
		{name: "empty", args: []string{"scan", "--cache-dir="}, want: "--cache-dir must not be empty"},
		{name: "missing", args: []string{"scan", "--cache-dir"}, want: "flag --cache-dir requires a value"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			args := append([]string{"--format=json"}, testCase.args...)

			reply := runGlobalArgumentsJSON(t, args, 2)
			if reply.OK || reply.Command != testCase.args[0] || len(reply.Diagnostics) != 1 {
				t.Fatalf("invalid usage response: %+v", reply)
			}

			diagnostic := reply.Diagnostics[0]
			if diagnostic.Code != model.CodeInvalidPolicy || diagnostic.Message != testCase.want {
				t.Fatalf("diagnostic=%+v; want %q", diagnostic, testCase.want)
			}

			if strings.Contains(string(reply.Data), "private-cache") || strings.Contains(diagnostic.Message, "private-cache") {
				t.Fatal("cache path leaked in usage response")
			}
		})
	}
}

func TestCLIAnalysisCacheScanContract(t *testing.T) {
	t.Parallel()

	root, directory, _ := callGraphFixture(t)
	relative := "analysis-cache"
	args := []string{"--root", root, "--offline", "--format=json", "--cache-dir", relative, "scan"}

	uncached := runGlobalArgumentsJSON(t, []string{"--root", root, "--offline", "--format=json", "scan"}, 0)
	first := runGlobalArgumentsJSON(t, args, 0)

	second := runGlobalArgumentsJSON(t, args, 0)
	if !bytes.Equal(uncached.Data, first.Data) || !bytes.Equal(first.Data, second.Data) {
		t.Fatalf("cached and uncached scan data differ: uncached=%s first=%s second=%s",
			uncached.Data, first.Data, second.Data)
	}

	cache := openCacheRoot(t, filepath.Join(root, relative))
	if entries := cacheEntries(t, cache); len(entries) == 0 {
		t.Fatal("scan did not populate the requested semantic cache")
	}

	_, err := cache.Stat(".")
	if err != nil {
		t.Fatalf("relative cache path was not rooted at project root: %v", err)
	}

	absolutePath := filepath.Join(t.TempDir(), "absolute-cache")
	absoluteArgs := []string{"--root", root, "--offline", "--format=json", "--cache-dir", absolutePath, "scan"}
	runGlobalArgumentsJSON(t, absoluteArgs, 0)

	_, err = os.Stat(absolutePath)
	if err != nil {
		t.Fatalf("absolute cache path was not preserved: %v", err)
	}

	err = directory.WriteFile("app.go", []byte("package app\n"+
		"import \"context\"\n"+
		"func Changed(ctx context.Context) error { return nil }\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	changed := runGlobalArgumentsJSON(t, args, 0)
	if bytes.Equal(second.Data, changed.Data) {
		t.Fatal("source change reused stale cached scan data")
	}
}

func TestCLIAnalysisCacheAcceptedCommands(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"scan", "init", "inspect", "explain", "validate", "lock", "diff"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			testAcceptedCommand(t, command)
		})
	}
}

func testAcceptedCommand(t *testing.T, command string) {
	t.Helper()

	root := acceptedCommandFixture(t, command)
	if command == "lock" || command == "diff" {
		args := []string{"--root", root, "--offline", "--format=json", "lock"}
		runGlobalArgumentsJSON(t, args, 0)
	}

	args := acceptedCommandArguments(root, command)

	reply := runGlobalArgumentsJSON(t, args, 0)
	if !reply.OK || reply.Command != command {
		t.Fatalf("invalid successful %s response: %+v", command, reply)
	}

	cachePath := filepath.Join(root, "analysis-cache")
	if command == "diff" {
		_, err := os.Stat(cachePath)
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("diff created read-only cache: %v", err)
		}

		return
	}

	cache := openCacheRoot(t, cachePath)
	if len(cacheFiles(t, cache)) == 0 {
		t.Fatalf("%s did not populate the requested semantic cache", command)
	}
}

func acceptedCommandFixture(t *testing.T, command string) string {
	t.Helper()

	if command == "scan" || command == "init" {
		root, _, _ := callGraphFixture(t)

		return root
	}

	root, _, _ := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n", executionPolicy)

	return root
}

func acceptedCommandArguments(root, command string) []string {
	args := make([]string, 0, 8)

	args = append(args, "--root", root, "--offline", "--format=json", "--cache-dir",
		filepath.Join(root, "analysis-cache"), command)
	if command == "init" {
		args = append(args, "--non-interactive", "--output", "starter.yaml")
	}

	if command == "explain" {
		args = append(args, "example.com/app.Run")
	}

	return args
}

func TestCLIAnalysisCacheStorageFailure(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"text", "json"} {
		for _, verbose := range []bool{false, true} {
			t.Run(format+"/verbose="+strconv.FormatBool(verbose), func(t *testing.T) {
				t.Parallel()

				testStorageFailure(t, format, verbose)
			})
		}
	}
}

func testStorageFailure(t *testing.T, format string, verbose bool) {
	t.Helper()

	root, directory, _ := callGraphFixture(t)
	cachePath := filepath.Join(root, "cache-file")

	err := directory.WriteFile("cache-file", []byte("regular file"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	args := storageFailureArguments(root, cachePath, format, verbose)

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), args, &stdout, &stderr)
	if exit != 4 || stderr.Len() != 0 {
		t.Fatalf("storage failure exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}

	assertStorageFailureResponse(t, format, stdout.Bytes(), cachePath)
}

func storageFailureArguments(root, cachePath, format string, verbose bool) []string {
	args := []string{"scan", "--root", root, "--offline", "--format=" + format, "--cache-dir", cachePath}
	if verbose {
		args = append(args, "--verbose")
	}

	return args
}

func assertStorageFailureResponse(t *testing.T, format string, output []byte, cachePath string) {
	t.Helper()

	text := string(output)
	if !strings.Contains(text, "cannot access requested analysis cache") || strings.Contains(text, cachePath) {
		t.Fatalf("storage diagnostic=%s; want static message without cache path", text)
	}

	if format != "json" {
		return
	}

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil {
		t.Fatalf("decode storage failure: %v", err)
	}

	if len(reply.Diagnostics) != 1 || reply.Diagnostics[0].Code != model.CodeUnresolvedSymbol {
		t.Fatalf("storage diagnostics=%+v", reply.Diagnostics)
	}
}

func TestCLIAnalysisCacheReadOnlyLockAndDiff(t *testing.T) {
	t.Parallel()

	root, _, _ := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n", executionPolicy)
	cachePath := filepath.Join(root, "analysis-cache")
	uncachedInspect := runGlobalArgumentsJSON(t, []string{"--root", root, "--offline", "--format=json", "inspect"}, 0)
	setupLock := []string{"--root", root, "--offline", "--format=json", "lock"}
	runGlobalArgumentsJSON(t, setupLock, 0)
	uncachedLock := runGlobalArgumentsJSON(t, setupLock, 0)
	cacheArgs := []string{"--root", root, "--offline", "--format=json", "--cache-dir", cachePath}
	cachedInspectArgs := append(append([]string{}, cacheArgs...), "inspect")
	cachedInspect := runGlobalArgumentsJSON(t, cachedInspectArgs, 0)

	cachedLockArgs := append(append([]string{}, cacheArgs...), "lock")

	cachedLock := runGlobalArgumentsJSON(t, cachedLockArgs, 0)
	if !bytes.Equal(uncachedInspect.Data, cachedInspect.Data) || !bytes.Equal(uncachedLock.Data, cachedLock.Data) {
		t.Fatalf("cached and uncached policy data differ: inspect=%t lock=%t uncached=%s cached=%s",
			!bytes.Equal(uncachedInspect.Data, cachedInspect.Data), !bytes.Equal(uncachedLock.Data, cachedLock.Data),
			uncachedLock.Data, cachedLock.Data)
	}

	before := testenv.SnapshotFiles(t, root)

	cache := openCacheRoot(t, cachePath)
	original := snapshotCache(t, cache)
	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))

	for _, suffix := range [][]string{{"lock", "--check"}, {"lock", "--dry-run"}, {"diff"}, {"diff", "--check"}} {
		args := []string{"--root", root, "--offline", "--format=json", "--cache-dir", cachePath}
		args = append(args, suffix...)
		runGlobalArgumentsJSON(t, args, 0)
		assertCacheUnchanged(t, original, snapshotCache(t, cache))
		testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))
	}

	entries := cacheFiles(t, cache)
	if len(entries) == 0 {
		t.Fatal("lock did not populate semantic cache")
	}

	err := cache.WriteFile(entries[0], []byte("{corrupt"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	corrupt := snapshotCache(t, cache)
	corruptBefore := testenv.SnapshotFiles(t, root)

	for _, suffix := range [][]string{{"lock", "--check"}, {"lock", "--dry-run"}, {"diff"}, {"diff", "--check"}} {
		args := make([]string, 0, 6+len(suffix))
		args = append(args, "--root", root, "--offline", "--format=json", "--cache-dir", cachePath)
		args = append(args, suffix...)
		runGlobalArgumentsJSON(t, args, 0)
		assertCacheUnchanged(t, corrupt, snapshotCache(t, cache))
		testenv.AssertFilesUnchanged(t, corruptBefore, testenv.SnapshotFiles(t, root))
	}
}

func TestCLIAnalysisCacheLockBytes(t *testing.T) {
	t.Parallel()

	root, project, _ := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n", executionPolicy)
	args := make([]string, 0, 7)

	args = append(args, "lock", "--root", root, "--offline", "--format=json")
	runGlobalArgumentsJSON(t, args, 0)

	before, err := project.ReadFile("otelplan.lock")
	if err != nil {
		t.Fatal(err)
	}

	args = append(args, "--cache-dir", filepath.Join(root, "analysis-cache"))
	runGlobalArgumentsJSON(t, args, 0)

	after, err := project.ReadFile("otelplan.lock")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(before, after) {
		t.Fatal("semantic caching changed deterministic resolution lock bytes")
	}
}

func TestCLIAnalysisCacheReadOnlyMissingPath(t *testing.T) {
	t.Parallel()

	root, _, _ := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n", executionPolicy)
	runGlobalArgumentsJSON(t, []string{"--root", root, "--offline", "--format=json", "lock"}, 0)
	before := testenv.SnapshotFiles(t, root)

	base := t.TempDir()

	cachePath := filepath.Join(base, "nested", "missing")

	for _, suffix := range [][]string{{"lock", "--check"}, {"lock", "--dry-run"}, {"diff"}, {"diff", "--check"}} {
		args := make([]string, 0, 6+len(suffix))
		args = append(args, "--root", root, "--offline", "--format=json", "--cache-dir", cachePath)
		args = append(args, suffix...)
		runGlobalArgumentsJSON(t, args, 0)
		testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))
	}

	_, err := os.Stat(filepath.Dir(cachePath))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only command created cache parent: %v", err)
	}
}

func openCacheRoot(t *testing.T, path string) *os.Root {
	t.Helper()

	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	return root
}

func cacheEntries(t *testing.T, root *os.Root) []string {
	t.Helper()

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		t.Fatal(err)
	}

	names := make([]string, 0, len(entries))

	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

func cacheFiles(t *testing.T, root *os.Root) []string {
	t.Helper()

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		t.Fatal(err)
	}

	names := make([]string, 0, len(entries))

	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}

	return names
}

func snapshotCache(t *testing.T, root *os.Root) map[string][]byte {
	t.Helper()

	snapshot := make(map[string][]byte)

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			snapshot[name] = nil

			continue
		}

		contents, err := root.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}

		snapshot[name] = contents
	}

	return snapshot
}

func assertCacheUnchanged(t *testing.T, want, got map[string][]byte) {
	t.Helper()

	if len(want) != len(got) {
		t.Fatalf("cache entry count changed: want %d, got %d", len(want), len(got))
	}

	for name, contents := range want {
		gotContents, exists := got[name]
		if !exists || !bytes.Equal(contents, gotContents) {
			t.Fatalf("cache entry %s changed", name)
		}
	}
}
