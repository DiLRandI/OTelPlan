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
	prepareOfflineIntegration(t)
	root, originals := cliFixture(t)

	project, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := project.Close()
		if closeErr != nil {
			t.Errorf("close project root: %v", closeErr)
		}
	})

	for _, format := range []string{"json", "text"} {
		verifyCompileOutput(t, root, format)
	}

	verifyCompiledCLIArtifacts(t, root, originals, project)
}

func verifyCompileOutput(t *testing.T, root, format string) {
	t.Helper()

	var out, errout bytes.Buffer

	args := []string{"compile", "--root", root, "--format", format, "--offline", "--verbose"}

	code := Run(t.Context(), args, &out, &errout)
	if code != 0 {
		t.Fatalf("compile exit=%d: %s %s", code, &out, &errout)
	}

	if format == "json" {
		var reply struct {
			OK   bool           `json:"ok"`
			Data compileSummary `json:"data"`
		}

		err := json.Unmarshal(out.Bytes(), &reply)
		if err != nil {
			t.Fatalf("decode compile response: %v", err)
		}

		if !reply.OK || reply.Data.Backend.Digest == "" || reply.Data.Files == 0 {
			t.Fatalf("invalid compile response: %s", &out)
		}

		return
	}

	if !strings.Contains(out.String(), "artifacts=") || !strings.Contains(out.String(), "BUILD ") {
		t.Fatalf("missing text compile summary: %s", &out)
	}
}

func verifyCompiledCLIArtifacts(t *testing.T, root string, originals map[string]string, project *os.Root) {
	t.Helper()

	binaryDir := t.TempDir()
	binary := filepath.Join(binaryDir, "otelplan")
	buildCLI(t, binary)

	for _, args := range [][]string{
		{"compile", "--root", root, "--format=json", "--offline"},
		{"compile", "--root", root, "--output", "custom-build", "--format=json", "--offline"},
	} {
		command := exec.CommandContext(t.Context(), "./otelplan", args...)
		command.Dir = binaryDir

		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("compile executable with %v: %v %s", args, err, output)
		}
	}

	verifyPublishedArtifacts(t, root)
	verifySourcesAndLock(t, originals, project)
	verifyCleanRetainsUnrelatedFile(t, root, project)
}

func buildCLI(t *testing.T, binary string) {
	t.Helper()

	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "../../cmd/otelplan")

	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build CLI: %v %s", err, output)
	}
}

func verifyPublishedArtifacts(t *testing.T, root string) {
	t.Helper()

	_, err := compiler.ReadArtifacts(filepath.Join(root, ".otelplan", "build"))
	if err != nil {
		t.Fatalf("read default CLI artifacts: %v", err)
	}

	_, err = compiler.ReadArtifacts(filepath.Join(root, "custom-build"))
	if err != nil {
		t.Fatalf("read custom CLI artifacts: %v", err)
	}
}

func verifySourcesAndLock(t *testing.T, originals map[string]string, project *os.Root) {
	t.Helper()

	for name, want := range originals {
		data, err := project.ReadFile(name)
		if err != nil {
			t.Fatalf("read source file %s: %v", name, err)
		}

		if string(data) != want {
			t.Fatalf("changed application file %s", name)
		}
	}

	_, err := project.Stat("otelplan.lock")
	if !os.IsNotExist(err) {
		t.Fatal("compile changed resolution lock")
	}
}

func verifyCleanRetainsUnrelatedFile(t *testing.T, root string, project *os.Root) {
	t.Helper()

	err := project.WriteFile(".otelplan/build/unrelated", []byte("keep"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	var out, errout bytes.Buffer

	args := []string{"compile", "--root", root, "--clean", "--format=json", "--offline"}

	code := Run(t.Context(), args, &out, &errout)
	if code != 1 {
		t.Fatalf("clean exit=%d: %s", code, &out)
	}

	data, err := project.ReadFile(".otelplan/build/unrelated")
	if err != nil || string(data) != "keep" {
		t.Fatal("clean removed unrelated output")
	}
}

func TestCompileCLIUsage(t *testing.T) {
	t.Parallel()

	invalidArgs := [][]string{
		{"compile", "extra"},
		{"compile", "--output="},
		{"inspect", "--output=build"},
		{"scan", "--clean"},
		{"compile", "--check"},
	}
	for _, args := range invalidArgs {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()

			for _, format := range []string{"text", "json"} {
				var out, errout bytes.Buffer
				if code := Run(t.Context(), append(args, "--format="+format), &out, &errout); code != 2 {
					t.Fatalf("%v: exit=%d %s %s", args, code, &out, &errout)
				}

				if format == "json" {
					var reply response

					err := json.Unmarshal(out.Bytes(), &reply)
					if err != nil || reply.OK || len(reply.Diagnostics) == 0 {
						t.Fatalf("invalid error JSON: %s", &out)
					}
				} else if out.Len() == 0 && errout.Len() == 0 {
					t.Fatal("text usage error did not include a diagnostic")
				}
			}
		})
	}
}

func TestCompileOfflineDisablesProxyBypass(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required for pinned backend integration")
	}

	prepareOfflineIntegration(t)

	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}

	wrapperDir := t.TempDir()
	marker := filepath.Join(wrapperDir, "checked")

	wrapperRoot, err := os.OpenRoot(wrapperDir)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := wrapperRoot.Close()
		if closeErr != nil {
			t.Errorf("close wrapper root: %v", closeErr)
		}
	})

	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

	script := strings.Join([]string{
		"#!/bin/sh",
		"if [ \"$1\" = test ]; then",
		" [ \"$GOPROXY\" = off ] && [ \"$GONOPROXY\" = none ] && [ \"$GOSUMDB\" = off ] || exit 91",
		" touch " + quote(marker),
		"fi",
		"exec " + quote(realGo) + " \"$@\"",
		"",
	}, "\n")

	err = wrapperRoot.WriteFile("go", []byte(script), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = wrapperRoot.Chmod("go", 0o700)
	if err != nil {
		t.Fatalf("make Go wrapper executable: %v", err)
	}

	path := strings.Join([]string{wrapperDir, filepath.Dir(executable), os.Getenv("PATH")}, string(os.PathListSeparator))
	t.Setenv("PATH", path)
	t.Setenv("GONOPROXY", "*")
	root, _ := cliFixture(t)

	var out, errout bytes.Buffer

	args := []string{"compile", "--root", root, "--offline", "--format=json"}

	code := Run(t.Context(), args, &out, &errout)
	if code != 0 {
		t.Fatalf("offline compile exit=%d: %s %s", code, &out, &errout)
	}

	_, err = wrapperRoot.Stat("checked")
	if err != nil {
		t.Fatal("generated-source compilation did not enforce offline environment")
	}
}
