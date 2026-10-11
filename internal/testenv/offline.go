// Package testenv prepares isolated Go dependencies for offline integration tests.
package testenv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

const (
	cleanupTimeout = 30 * time.Second
	manifestMode   = 0o600
)

// ConfigureGoCache installs the integration Go environment and owns cache cleanup.
// It uses t.Setenv, so callers must run serially.
func ConfigureGoCache(t *testing.T, cache string) {
	t.Helper()

	t.Setenv("GOENV", "off")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOPACKAGESDRIVER", "off")
	t.Setenv("GOPROXY", "https://proxy.golang.org")
	t.Setenv("GOSUMDB", "sum.golang.org")
	t.Setenv("GOPRIVATE", "")
	t.Setenv("GONOPROXY", "none")
	t.Setenv("GONOSUMDB", "none")
	t.Setenv("GOVCS", "*:off")
	t.Setenv("GOTMPDIR", t.TempDir())

	t.Cleanup(func() {
		// Test contexts are canceled before cleanup; removing read-only module caches still needs a live context.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), cleanupTimeout)
		defer cancel()

		command := exec.CommandContext(ctx, "go", "clean", "-modcache")

		command.Env = append(os.Environ(), "GOMODCACHE="+cache)

		output, err := command.CombinedOutput()
		if err != nil {
			t.Errorf("clean integration module cache: %v: %s", err, output)
		}
	})
}

// DenyNetwork disables module lookups and fails if the test uses an HTTP proxy.
// It uses t.Setenv, so callers must run serially.
func DenyNetwork(t *testing.T) {
	t.Helper()

	var requests atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)

		writer.WriteHeader(http.StatusServiceUnavailable)
	}))

	t.Cleanup(func() {
		server.Close()

		if count := requests.Load(); count != 0 {
			t.Errorf("offline integration workflow made %d network requests", count)
		}
	})

	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("HTTP_PROXY", server.URL)
	t.Setenv("HTTPS_PROXY", server.URL)
	t.Setenv("ALL_PROXY", server.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("http_proxy", server.URL)
	t.Setenv("https_proxy", server.URL)
	t.Setenv("all_proxy", server.URL)
	t.Setenv("no_proxy", "")
}

// PrepareOffline seeds a private dependency cache from copied module manifests,
// then denies network access. Source modules are read without modification.
// It uses t.Setenv, so callers must run serially.
func PrepareOffline(t *testing.T, modules ...string) {
	t.Helper()
	cache := t.TempDir()
	t.Setenv("GOMODCACHE", cache)
	ConfigureGoCache(t, cache)

	for _, source := range modules {
		prepareModule(t, source)
	}

	DenyNetwork(t)
}

func prepareModule(t *testing.T, source string) {
	t.Helper()

	original := openDirectory(t, source)
	seed := t.TempDir()
	destination := openDirectory(t, seed)

	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := original.ReadFile(name)
		if err != nil {
			t.Fatalf("read integration dependency manifest %s: %v", name, err)
		}

		err = destination.WriteFile(name, data, manifestMode)
		if err != nil {
			t.Fatalf("write integration dependency manifest %s: %v", name, err)
		}
	}

	command := exec.CommandContext(t.Context(), "go", "mod", "download", "all")
	command.Dir = seed

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare integration dependencies: %v: %s", err, output)
	}
}

func openDirectory(t *testing.T, directory string) *os.Root {
	t.Helper()

	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatalf("open integration directory: %v", err)
	}

	t.Cleanup(func() {
		err := root.Close()
		if err != nil {
			t.Errorf("close integration directory: %v", err)
		}
	})

	return root
}
