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
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&source, "func Function%03d(ctx context.Context) error { return nil }\n", i)
	}
	root := b.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/fixture\n\ngo 1.27\n"), 0644); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "symbols.go"), []byte(source.String()), 0644); err != nil {
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
	for i := 0; i < b.N; i++ {
		loaded, err := Load(opts)
		if err != nil {
			b.Fatal(err)
		}
		if len(loaded.Symbols) != len(warm.Symbols) {
			b.Fatalf("loaded %d symbols, want %d", len(loaded.Symbols), len(warm.Symbols))
		}
	}
}
