package discovery_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestRuntimeTraitsInvalidateOlderCacheEntries(t *testing.T) {
	t.Parallel()
	options, first, cache := cachedRuntimeTraitsFixture(t)
	entryName, old := makeTraitlessCacheEntry(t, cache)
	options.CacheReadOnly = true
	assertRuntimeTraitsAnalysis(t, options, first)

	after, err := cache.ReadFile(entryName)
	if err != nil || !bytes.Equal(old, after) {
		t.Fatal("read-only refresh changed old cache")
	}

	options.CacheReadOnly = false
	assertRuntimeTraitsAnalysis(t, options, first)
	assertRuntimeTraitsAnalysis(t, options, first)
}

func cachedRuntimeTraitsFixture(t *testing.T) (discovery.Options, *model.CodeModel, *os.Root) {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := root.Close()
		if err != nil {
			t.Error(err)
		}
	})

	for name, data := range map[string]string{"go.mod": "module example.com/cached\n\ngo 1.27\n", "app.go": `package app
import "net/http"
type Value string
func (Value) String() string { panic("must not execute") }
func Run(Value) {}
func recover() any { panic("must not execute") }
func Handler(http.ResponseWriter, *http.Request) {}
func register() { http.HandleFunc("/must-not-capture", Handler) }
`} {
		err := root.WriteFile(name, []byte(data), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	var options discovery.Options

	options.Root, options.Offline, options.CacheDir = root.Name(), true, filepath.Join(t.TempDir(), "cache")

	first, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	assertRuntimeTraits(t, first)

	cache, err := os.OpenRoot(options.CacheDir)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := cache.Close()
		if err != nil {
			t.Error(err)
		}
	})

	return options, first, cache
}

func assertRuntimeTraitsAnalysis(t *testing.T, options discovery.Options, want *model.CodeModel) {
	t.Helper()

	got, err := discovery.LoadContext(t.Context(), options)
	if err != nil || !reflect.DeepEqual(want, got) {
		t.Fatalf("cached runtime safety traits changed: %v", err)
	}
}

func makeTraitlessCacheEntry(t *testing.T, cache *os.Root) (string, []byte) {
	t.Helper()

	entries, err := fs.ReadDir(cache.FS(), ".")
	if err != nil || len(entries) != 1 {
		t.Fatal("analysis cache entry missing")
	}

	data, err := cache.ReadFile(entries[0].Name())
	if err != nil {
		t.Fatal(err)
	}

	var entry struct {
		Version int             `json:"version"`
		Key     string          `json:"key"`
		Digest  string          `json:"digest"`
		Model   json.RawMessage `json:"model"`
	}

	err = json.Unmarshal(data, &entry)
	if err != nil {
		t.Fatal(err)
	}

	var code model.CodeModel

	err = json.Unmarshal(entry.Model, &code)
	if err != nil {
		t.Fatal(err)
	}

	clearRuntimeTraits(&code)

	entry.Version--

	entry.Model, err = json.Marshal(code)
	if err != nil {
		t.Fatal(err)
	}

	digest := sha256.Sum256(entry.Model)
	entry.Digest = hex.EncodeToString(digest[:])

	data, err = json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}

	err = cache.WriteFile(entries[0].Name(), data, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return entries[0].Name(), data
}

func hasStringerType(code *model.CodeModel) bool {
	for _, typ := range code.Types {
		if typ.Stringer {
			return true
		}
	}

	return false
}

func assertRuntimeTraits(t *testing.T, code *model.CodeModel) {
	t.Helper()

	symbol, exists := code.Symbol(model.FunctionID("example.com/cached", "Run"))
	if !hasStringerType(code) || !exists || !symbol.RecoverShadowed {
		t.Fatal("fresh analysis lacks runtime safety traits")
	}

	handler, found := code.Symbol(model.FunctionID("example.com/cached", "Handler"))
	if !found || len(handler.Entrypoints) != 1 || handler.Entrypoints[0] != "net/http.HandleFunc" {
		t.Fatal("fresh analysis lacks technical entrypoint metadata")
	}
}

func clearRuntimeTraits(code *model.CodeModel) {
	for index := range code.Types {
		code.Types[index].Stringer = false
	}

	for index := range code.Symbols {
		code.Symbols[index].RecoverShadowed = false
		code.Symbols[index].Entrypoints = nil
	}
}
