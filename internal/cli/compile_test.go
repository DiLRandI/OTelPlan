package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
)

func TestCompileCLI(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required for pinned backend integration")
	}
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root, originals := cliFixture(t)
	for _, format := range []string{"json", "text"} {
		var out, errout bytes.Buffer
		code := Run([]string{"compile", "--root", root, "--format", format, "--offline"}, &out, &errout)
		if code != 0 {
			t.Fatalf("compile exit=%d: %s %s", code, &out, &errout)
		}
		if format == "json" {
			var reply struct {
				OK   bool           `json:"ok"`
				Data compileSummary `json:"data"`
			}
			if err := json.Unmarshal(out.Bytes(), &reply); err != nil || !reply.OK || reply.Data.Backend.Digest == "" || reply.Data.Files == 0 {
				t.Fatalf("invalid compile response: %s", &out)
			}
		} else if !strings.Contains(out.String(), "artifacts=") {
			t.Fatalf("missing text summary: %s", &out)
		}
	}
	binary := filepath.Join(t.TempDir(), "otelplan")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/otelplan")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v %s", err, output)
	}
	command := exec.Command(binary, "compile", "--root", root, "--output", "custom-build", "--format=json", "--offline")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("compile executable: %v %s", err, output)
	}
	if _, err := compiler.ReadArtifacts(filepath.Join(root, "custom-build")); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, ".otelplan", "build")
	if _, err := compiler.ReadArtifacts(output); err != nil {
		t.Fatal(err)
	}
	for name, want := range originals {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(data) != want {
			t.Fatalf("changed application file %s", name)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "otelplan.lock")); !os.IsNotExist(err) {
		t.Fatal("compile changed resolution lock")
	}
	if err := os.WriteFile(filepath.Join(output, "unrelated"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	if code := Run([]string{"compile", "--root", root, "--clean", "--format=json", "--offline"}, &out, &errout); code != 1 {
		t.Fatalf("clean exit=%d: %s", code, &out)
	}
	data, err := os.ReadFile(filepath.Join(output, "unrelated"))
	if err != nil || string(data) != "keep" {
		t.Fatal("clean removed unrelated output")
	}
}

func TestCompileCLIUsage(t *testing.T) {
	for _, args := range [][]string{{"compile", "extra"}, {"compile", "--output="}, {"inspect", "--output=build"}, {"scan", "--clean"}, {"compile", "--check"}} {
		for _, format := range []string{"text", "json"} {
			var out, errout bytes.Buffer
			if code := Run(append(args, "--format="+format), &out, &errout); code != 2 {
				t.Fatalf("%v: exit=%d %s %s", args, code, &out, &errout)
			}
			if format == "json" {
				var reply response
				if err := json.Unmarshal(out.Bytes(), &reply); err != nil || reply.OK || len(reply.Diagnostics) == 0 {
					t.Fatalf("invalid error JSON: %s", &out)
				}
			}
		}
	}
}
