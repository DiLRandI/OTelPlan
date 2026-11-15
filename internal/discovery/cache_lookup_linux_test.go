package discovery_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
)

const cacheLookupTimeout = 2 * time.Second

func TestCacheLookupRejectsNonregularSHAEntries(t *testing.T) {
	t.Parallel()

	for _, entryKind := range []string{"directory", "fifo"} {
		t.Run(entryKind, func(t *testing.T) {
			t.Parallel()

			for _, readOnly := range []bool{false, true} {
				name := "writable"
				if readOnly {
					name = "read-only"
				}

				t.Run(name, func(t *testing.T) {
					t.Parallel()
					testNonregularCacheEntry(t, entryKind, readOnly)
				})
			}
		})
	}
}

func testNonregularCacheEntry(t *testing.T, entryKind string, readOnly bool) {
	t.Helper()

	opts, entryPath, source := warmCacheLookupFixture(t)
	replaceCacheLookupEntry(t, entryPath, entryKind)

	opts.CacheReadOnly = readOnly
	lookup := startCacheLookup(t, opts)

	var lookupErr error
	if entryKind == "fifo" {
		lookupErr = awaitFIFOCacheLookup(t, entryPath, lookup)
	} else {
		lookupErr = awaitCacheLookup(t, lookup)
	}

	if !errors.Is(lookupErr, discovery.ErrCacheStorage) {
		t.Fatalf("lookup error=%v; want errors.Is(..., ErrCacheStorage)", lookupErr)
	}

	assertCacheLookupEntry(t, entryPath, entryKind)
	assertCacheLookupSource(t, opts.Root, source)
}

func warmCacheLookupFixture(t *testing.T) (discovery.Options, string, []byte) {
	t.Helper()

	root := t.TempDir()
	source := []byte("package shop\nfunc Original() {}\n")

	writeCacheLookupFile(t, root, "go.mod", "module example.com/shop\n\ngo 1.27\n")
	writeCacheLookupFile(t, root, "app.go", string(source))

	opts := new(discovery.Options)
	opts.Root, opts.CacheDir, opts.Offline = root, filepath.Join(t.TempDir(), "cache"), true
	opts.Env = []string{"GOWORK=off", "GOFLAGS=", "GOENV=off", "CGO_ENABLED=0"}

	_, err := discovery.LoadContext(t.Context(), *opts)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(opts.CacheDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("cache entries=%v, err=%v; want one entry", entries, err)
	}

	return *opts, filepath.Join(opts.CacheDir, entries[0].Name()), source
}

func replaceCacheLookupEntry(t *testing.T, path, entryKind string) {
	t.Helper()

	err := os.Remove(path)
	if err != nil {
		t.Fatal(err)
	}

	switch entryKind {
	case "directory":
		err = os.Mkdir(path, 0o700)
	case "fifo":
		err = syscall.Mkfifo(path, 0o600)
	default:
		t.Fatalf("unknown cache entry kind %q", entryKind)
	}

	if err != nil {
		t.Fatal(err)
	}
}

func assertCacheLookupEntry(t *testing.T, path, entryKind string) {
	t.Helper()

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}

	switch entryKind {
	case "directory":
		if !info.IsDir() {
			t.Fatalf("cache entry mode=%s; want directory", info.Mode())
		}
	case "fifo":
		if info.Mode()&os.ModeNamedPipe == 0 {
			t.Fatalf("cache entry mode=%s; want FIFO", info.Mode())
		}
	}
}

func assertCacheLookupSource(t *testing.T, root string, want []byte) {
	t.Helper()

	directory, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		closeErr := directory.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	}()

	got, err := directory.ReadFile("app.go")
	if err != nil || string(got) != string(want) {
		t.Fatalf("source changed: err=%v, contents=%q", err, got)
	}
}

func writeCacheLookupFile(t *testing.T, root, name, contents string) {
	t.Helper()

	err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o700)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600)
	if err != nil {
		t.Fatal(err)
	}
}

func startCacheLookup(t *testing.T, opts discovery.Options) <-chan error {
	t.Helper()

	result := make(chan error, 1)

	go func() {
		_, err := discovery.LoadContext(t.Context(), opts)
		result <- err
	}()

	return result
}

func awaitCacheLookup(t *testing.T, result <-chan error) error {
	t.Helper()

	timer := time.NewTimer(cacheLookupTimeout)
	defer timer.Stop()

	select {
	case err := <-result:
		return err
	case <-timer.C:
		t.Fatal("cache lookup did not return promptly")

		return nil
	}
}

func awaitFIFOCacheLookup(t *testing.T, path string, result <-chan error) error {
	t.Helper()

	timer := time.NewTimer(cacheLookupTimeout)
	defer timer.Stop()

	select {
	case err := <-result:
		return err
	case <-timer.C:
		t.Error("FIFO cache lookup did not return promptly")
		releaseFIFOReader(t, path)
	}

	return awaitCacheLookup(t, result)
}

func releaseFIFOReader(t *testing.T, path string) {
	t.Helper()

	directory, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		closeErr := directory.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	}()

	deadline := time.Now().Add(cacheLookupTimeout)

	for {
		writer, err := directory.OpenFile(filepath.Base(path), os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			closeErr := writer.Close()
			if closeErr != nil {
				t.Fatal(closeErr)
			}

			return
		}

		if !errors.Is(err, syscall.ENXIO) || time.Now().After(deadline) {
			t.Fatalf("release FIFO reader: %v", err)
		}

		time.Sleep(10 * time.Millisecond)
	}
}
