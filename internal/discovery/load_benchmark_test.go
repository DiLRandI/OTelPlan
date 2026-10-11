package discovery_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
)

func BenchmarkLoadGeneratedSymbols(b *testing.B) {
	var source strings.Builder
	source.WriteString("package fixture\n\nimport \"context\"\n\n")

	for i := range 100 {
		fmt.Fprintf(&source, "func Function%03d(ctx context.Context) error { return nil }\n", i)
	}

	root := b.TempDir()

	directory, err := os.OpenRoot(root)
	if err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() {
		closeErr := directory.Close()
		if closeErr != nil {
			b.Error(closeErr)
		}
	})
	writeBenchmarkFile(b, directory, "go.mod", "module example.com/fixture\n\ngo 1.27\n")
	writeBenchmarkFile(b, directory, "symbols.go", source.String())

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Patterns = []string{"./..."}
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	warm, err := discovery.LoadContext(b.Context(), options)
	if err != nil {
		b.Fatal(err)
	}

	if len(warm.Symbols) != 100 {
		b.Fatalf("fixture loaded %d symbols, want 100", len(warm.Symbols))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		loaded, err := discovery.LoadContext(b.Context(), options)
		if err != nil {
			b.Fatal(err)
		}

		if len(loaded.Symbols) != len(warm.Symbols) {
			b.Fatalf("loaded %d symbols, want %d", len(loaded.Symbols), len(warm.Symbols))
		}
	}
}

func writeBenchmarkFile(b *testing.B, directory *os.Root, name, contents string) {
	b.Helper()

	err := directory.WriteFile(name, []byte(contents), 0o600)
	if err != nil {
		b.Fatal(err)
	}
}
