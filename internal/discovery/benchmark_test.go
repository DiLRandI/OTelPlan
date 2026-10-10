package discovery

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

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

	b.Cleanup(func() {
		closeErr := directory.Close()
		if closeErr != nil {
			b.Error(closeErr)
		}
	})

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
