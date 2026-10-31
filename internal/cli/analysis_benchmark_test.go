package cli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const (
	analysisBenchmarkPackages  = 100
	analysisBenchmarkFunctions = 20
	analysisBenchmarkModule    = "example.com/analysisbench"
)

func BenchmarkCLIStaticAnalysis(b *testing.B) {
	b.Setenv("GOENV", "off")
	b.Setenv("GOWORK", "off")
	b.Setenv("GOFLAGS", "")
	b.Setenv("GOTOOLCHAIN", "local")
	b.Setenv("GOPACKAGESDRIVER", "off")
	b.Setenv("CGO_ENABLED", "0")

	root, inputs := analysisBenchmarkFixture(b)
	args := []string{"scan", "--root", root.Name(), "--offline", "--format=json", "./..."}
	baseline := analysisBenchmarkPreview(b, args)
	checkAnalysisBenchmarkInventory(b, baseline)

	for _, mode := range []string{"uncached", "cached"} {
		b.Run(mode, func(b *testing.B) {
			current := append([]string{}, args...)

			cacheDir := ""
			if mode == "cached" {
				cacheDir = b.TempDir()
				current = append(current, "--cache-dir", cacheDir)
			}

			first := analysisBenchmarkPreview(b, current)

			second := analysisBenchmarkPreview(b, current)
			if !bytes.Equal(baseline, first) || !bytes.Equal(first, second) {
				b.Fatal("cached, uncached, or repeated scan output differs")
			}

			checkAnalysisBenchmarkCache(b, cacheDir)

			var stderr bytes.Buffer

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				stderr.Reset()

				exit := cli.Run(b.Context(), current, io.Discard, &stderr)
				if exit != 0 || stderr.Len() != 0 {
					b.Fatalf("scan failed: exit=%d stderr=%s", exit, &stderr)
				}
			}

			b.StopTimer()
			checkAnalysisBenchmarkInputs(b, root, inputs)
		})
	}
}

func analysisBenchmarkFixture(b *testing.B) (*os.Root, map[string][sha256.Size]byte) {
	b.Helper()

	root, err := os.OpenRoot(b.TempDir())
	if err != nil {
		b.Fatalf("open analysis benchmark fixture: %v", err)
	}

	b.Cleanup(func() {
		closeErr := root.Close()
		if closeErr != nil {
			b.Errorf("close analysis benchmark fixture: %v", closeErr)
		}
	})

	inputs := map[string][sha256.Size]byte{}
	writeAnalysisBenchmarkInput(b, root, inputs, "go.mod", "module "+analysisBenchmarkModule+"\n\ngo 1.27\n")

	for index := range analysisBenchmarkPackages {
		directory := fmt.Sprintf("pkg%03d", index)

		mkdirErr := root.MkdirAll(directory, 0o700)
		if mkdirErr != nil {
			b.Fatalf("create benchmark package: %v", mkdirErr)
		}

		var source strings.Builder

		fmt.Fprintf(&source, "package %s\nimport \"context\"\n", directory)

		for function := range analysisBenchmarkFunctions {
			fmt.Fprintf(&source, "func Operation%03d(ctx context.Context) error { return ctx.Err() }\n", function)
		}

		writeAnalysisBenchmarkInput(b, root, inputs, filepath.Join(directory, "operations.go"), source.String())
	}

	return root, inputs
}

func writeAnalysisBenchmarkInput(b *testing.B, root *os.Root, inputs map[string][sha256.Size]byte,
	name, source string) {
	b.Helper()

	data := []byte(source)

	err := root.WriteFile(name, data, 0o600)
	if err != nil {
		b.Fatalf("write analysis benchmark input %s: %v", name, err)
	}

	inputs[name] = sha256.Sum256(data)
}

func analysisBenchmarkPreview(b *testing.B, args []string) []byte {
	b.Helper()

	var stdout, stderr bytes.Buffer

	exit := cli.Run(b.Context(), args, &stdout, &stderr)
	if exit != 0 || stderr.Len() != 0 {
		b.Fatalf("analysis benchmark preflight: exit=%d stderr=%s stdout=%s", exit, &stderr, &stdout)
	}

	return bytes.Clone(stdout.Bytes())
}

func checkAnalysisBenchmarkInventory(b *testing.B, output []byte) {
	b.Helper()

	var reply struct {
		OK   bool            `json:"ok"`
		Data model.CodeModel `json:"data"`
	}

	err := json.Unmarshal(output, &reply)
	if err != nil {
		b.Fatalf("decode analysis benchmark inventory: %v", err)
	}

	want := analysisBenchmarkPackages * analysisBenchmarkFunctions
	if !reply.OK || len(reply.Data.Packages) != analysisBenchmarkPackages || len(reply.Data.Symbols) != want {
		b.Fatalf("incomplete analysis benchmark inventory: ok=%t packages=%d symbols=%d",
			reply.OK, len(reply.Data.Packages), len(reply.Data.Symbols))
	}

	for packageIndex := range analysisBenchmarkPackages {
		for function := range analysisBenchmarkFunctions {
			name := fmt.Sprintf("%s/pkg%03d.Operation%03d", analysisBenchmarkModule, packageIndex, function)
			if _, found := reply.Data.Symbol(model.SymbolID(name)); !found {
				b.Fatalf("analysis benchmark omitted %s", name)
			}
		}
	}
}

func checkAnalysisBenchmarkCache(b *testing.B, directory string) {
	b.Helper()

	if directory == "" {
		return
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		b.Fatalf("inspect requested analysis cache: %v", err)
	}

	if len(entries) == 0 {
		b.Fatal("cached scan did not populate the requested cache")
	}
}

func checkAnalysisBenchmarkInputs(b *testing.B, root *os.Root, inputs map[string][sha256.Size]byte) {
	b.Helper()

	current := map[string][sha256.Size]byte{}

	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk benchmark inputs: %w", walkErr)
		}

		if entry.IsDir() {
			return nil
		}

		data, readErr := root.ReadFile(name)
		if readErr != nil {
			return fmt.Errorf("read benchmark input %s: %w", name, readErr)
		}

		current[name] = sha256.Sum256(data)

		return nil
	})
	if err != nil {
		b.Fatal(err)
	}

	if !maps.Equal(current, inputs) {
		b.Fatal("analysis benchmark added, removed, or changed fixture inputs")
	}
}
