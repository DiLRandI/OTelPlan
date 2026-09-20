package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestScanAdvisoryCallGraph(t *testing.T) {
	t.Parallel()

	root, directory, original := callGraphFixture(t)

	for _, format := range []string{"text", "json"} {
		var out, errout bytes.Buffer

		args := []string{"scan", "--root", root, "--offline", "--calls", "--format=" + format}
		if exit := cli.Run(args, &out, &errout); exit != 0 {
			t.Fatalf("exit=%d: %s %s", exit, &out, &errout)
		}

		if format == "text" {
			checkCallGraphText(t, out.String())
		} else {
			checkCallGraphJSON(t, out.Bytes())
		}
	}

	for name, want := range original {
		got, err := directory.ReadFile(name)
		if err != nil || string(got) != want {
			t.Fatalf("scan changed %s", name)
		}
	}

	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != len(original) {
		t.Fatal("scan wrote project files")
	}
}

func checkCallGraphText(t *testing.T, output string) {
	t.Helper()

	for _, want := range []string{
		"CALLGRAPH cha conservative=true",
		"Reflection",
		"CALL static example.com/app.Caller -> example.com/app.Run",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q: %s", want, output)
		}
	}
}

func checkCallGraphJSON(t *testing.T, output []byte) {
	t.Helper()

	var reply struct {
		APIVersion string          `json:"apiVersion"`
		OK         bool            `json:"ok"`
		Data       model.CodeModel `json:"data"`
	}

	err := json.Unmarshal(output, &reply)
	if err != nil {
		t.Fatalf("decode graph response: %v", err)
	}

	if !reply.OK || reply.APIVersion != cli.APIVersion || reply.Data.CallGraph == nil {
		t.Fatalf("invalid graph response: %s", output)
	}

	if len(reply.Data.CallEdges) != 1 || reply.Data.CallEdges[0].Precision != model.CallPrecisionStatic {
		t.Fatalf("unexpected calls: %+v", reply.Data.CallEdges)
	}
}

func TestCallsFlagRequiresScan(t *testing.T) {
	t.Parallel()

	var out, errout bytes.Buffer
	if exit := cli.Run([]string{"inspect", "--calls", "--format=json"}, &out, &errout); exit != 2 {
		t.Fatalf("invalid applicability exit=%d: %s", exit, &out)
	}
}

func callGraphFixture(t *testing.T) (string, *os.Root, map[string]string) {
	t.Helper()

	root := t.TempDir()

	directory, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := directory.Close()
		if err != nil {
			t.Error(err)
		}
	})

	files := map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.27\n",
		"app.go": `package app
import "context"
func Run(ctx context.Context) error { return nil }
func Caller(ctx context.Context) error { return Run(ctx) }
`,
	}

	for name, content := range files {
		err := directory.WriteFile(name, []byte(content), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	return root, directory, files
}
