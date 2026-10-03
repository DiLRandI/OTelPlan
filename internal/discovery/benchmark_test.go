package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkLoadGeneratedSymbols(b *testing.B) {
	var source strings.Builder

	source.WriteString("package fixture\n\nimport \"context\"\n\n")

	for i := range 100 {
		fmt.Fprintf(&source, "func Function%03d(ctx context.Context) error { return nil }\n", i)
	}

	root := b.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/fixture\n\ngo 1.27\n"), 0o644); err != nil {
		b.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "symbols.go"), []byte(source.String()), 0o644); err != nil {
		b.Fatal(err)
	}

	opts := Options{Root: root, Patterns: []string{"./..."}}

	warm, err := Load(opts)
	if err != nil {
		b.Fatal(err)
	}

	if len(warm.Symbols) != 100 {
		b.Fatalf("fixture loaded %d symbols, want 100", len(warm.Symbols))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		loaded, err := Load(opts)
		if err != nil {
			b.Fatal(err)
		}

		if len(loaded.Symbols) != len(warm.Symbols) {
			b.Fatalf("loaded %d symbols, want %d", len(loaded.Symbols), len(warm.Symbols))
		}
	}
}

func BenchmarkAnalysisCache(b *testing.B) {
	var source strings.Builder

	source.WriteString("package fixture\nimport \"context\"\n")

	const functions = 100
	for i := range functions {
		fmt.Fprintf(&source, "func Function%03d(ctx context.Context) error { return nil }\n", i)
	}

	root := cacheBenchmarkFixture(b, source.String())

	for _, mode := range []string{"uncached", "cached"} {
		b.Run(mode, func(b *testing.B) {
			opts := new(Options)
			opts.Root, opts.Offline = root, true

			opts.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}
			if mode == "cached" {
				opts.CacheDir = b.TempDir()
			}

			_, _, err := loadContextResult(b.Context(), *opts)
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				code, hit, err := loadContextResult(b.Context(), *opts)
				if err != nil || len(code.Symbols) != functions || hit != (mode == "cached") {
					b.Fatal("repeated analysis was incomplete or did not use the requested cache")
				}
			}
		})
	}
}

func cacheBenchmarkFixture(b *testing.B, source string) string {
	b.Helper()

	root := b.TempDir()

	directory, err := os.OpenRoot(root)
	if err != nil {
		b.Fatal(err)
	}

	defer func() { _ = directory.Close() }()

	err = directory.WriteFile("go.mod", []byte("module example.com/fixture\n\ngo 1.27\n"), cacheFileMode)
	if err != nil {
		b.Fatal(err)
	}

	err = directory.WriteFile("app.go", []byte(source), cacheFileMode)
	if err != nil {
		b.Fatal(err)
	}

	return root
}
