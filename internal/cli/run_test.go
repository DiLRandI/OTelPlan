package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func cliFixture(t *testing.T) (string, map[string]string) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":        "module example.com/app\n\ngo 1.27\n",
		"app.go":        "package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n",
		"otelplan.yaml": "apiVersion: otelplan.io/v1alpha1\nkind: InstrumentationPlan\nbackend: {name: otelc, version: v1.1.0}\nrules:\n- id: operation\n  match:\n    functions: [Run]\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root, files
}

func TestInspectionCommandsJSONAndImmutability(t *testing.T) {
	root, files := cliFixture(t)
	for _, args := range [][]string{{"scan", "./..."}, {"inspect"}, {"explain", "example.com/app.Run"}, {"version"}} {
		var stdout, stderr bytes.Buffer
		args = append([]string{"--root", root}, append(args, "--format=json")...)
		if code := Run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("%v exit=%d stderr=%s stdout=%s", args, code, &stderr, &stdout)
		}
		var result response
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.APIVersion != APIVersion || !result.OK || result.Data == nil || len(result.Diagnostics) != 0 {
			t.Fatalf("invalid response: %+v", result)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(files) {
		t.Fatal("commands created project files")
	}
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(got) != want {
			t.Fatalf("changed %s", name)
		}
	}
}

func TestCLIExitCodes(t *testing.T) {
	root, _ := cliFixture(t)
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"--unknown"}, 2}, {[]string{"--format=xml", "scan"}, 2}, {[]string{"explain"}, 2},
		{[]string{"--config=missing.yaml", "inspect"}, 3}, {[]string{"scan", "./missing"}, 4},
		{[]string{"explain", "example.com/app.Missing"}, 5},
	} {
		var out, errout bytes.Buffer
		got := Run(append([]string{"--root", root}, tc.args...), &out, &errout)
		if got != tc.code {
			t.Fatalf("%v exit=%d want=%d output=%s %s", tc.args, got, tc.code, &out, &errout)
		}
	}
}

func TestCLIExecutable(t *testing.T) {
	root, _ := cliFixture(t)
	binary := filepath.Join(t.TempDir(), "otelplan")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/otelplan")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	command := exec.Command(binary, "inspect", "--root", root, "--format=json")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect: %v %s", err, output)
	}
	var reply response
	if err := json.Unmarshal(output, &reply); err != nil || !reply.OK {
		t.Fatalf("invalid JSON: %s", output)
	}
	command = exec.Command(binary, "inspect", "--root", root, "--config=missing.yaml")
	err = command.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Fatalf("invalid policy process exit: %v", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestCLIOutputFailure(t *testing.T) {
	var stderr bytes.Buffer
	if code := Run([]string{"version", "--format=json"}, failingWriter{}, &stderr); code != 1 {
		t.Fatalf("output failure exit=%d", code)
	}
}

func TestInspectRejectsUnsafeCaptureWithoutPrintingConstant(t *testing.T) {
	root, files := cliFixture(t)
	contents := files["otelplan.yaml"] + "  attributes:\n  - key: password\n    from:\n      constant: do-not-print-this-secret\n"
	if err := os.WriteFile(filepath.Join(root, "otelplan.yaml"), []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	if code := Run([]string{"inspect", "--root", root, "--format=json"}, &out, &errout); code != 5 {
		t.Fatalf("unsafe inspection exit=%d output=%s", code, &out)
	}
	if strings.Contains(out.String(), "do-not-print-this-secret") {
		t.Fatal("unsafe constant printed")
	}
}
