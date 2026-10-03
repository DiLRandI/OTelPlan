package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
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
		err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644)
		if err != nil {
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
		if code := Run(t.Context(), args, &stdout, &stderr); code != 0 {
			t.Fatalf("%v exit=%d stderr=%s stdout=%s", args, code, &stderr, &stdout)
		}

		var result response

		err := json.Unmarshal(stdout.Bytes(), &result)
		if err != nil {
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
		{[]string{"--unknown"}, 2},
		{[]string{"--format=xml", "scan"}, 2},
		{[]string{"explain"}, 2},
		{[]string{"--config=missing.yaml", "inspect"}, 3},
		{[]string{"scan", "./missing"}, 4},
		{[]string{"explain", "example.com/app.Missing"}, 5},
	} {
		var out, errout bytes.Buffer

		got := Run(t.Context(), append([]string{"--root", root}, tc.args...), &out, &errout)
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
	if code := Run(t.Context(), []string{"version", "--format=json"}, failingWriter{}, &stderr); code != 1 {
		t.Fatalf("output failure exit=%d", code)
	}
}

func TestRunCancelsGoDiscovery(t *testing.T) {
	const cancellationTimeout = 5 * time.Second

	root, _ := cliFixture(t)
	binDir := t.TempDir()
	ready := filepath.Join(root, "ready")

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("find test executable: %v", err)
	}

	goCommand := filepath.Join(binDir, "go")
	if runtime.GOOS == "windows" {
		goCommand += ".exe"
	}

	err = os.Link(executable, goCommand)
	if err != nil {
		t.Fatalf("link blocking Go command: %v", err)
	}

	t.Setenv("OTELPLAN_TEST_BLOCKING_GO", "1")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var stdout, stderr bytes.Buffer

	finished := make(chan int, 1)
	go func() {
		finished <- Run(ctx, []string{"scan", "--root", root, "--format=json"}, &stdout, &stderr)
	}()

	waitForBlockingGo(t, ready, finished, &stdout, &stderr)

	cancel()

	timeout := time.NewTimer(cancellationTimeout)
	defer timeout.Stop()

	select {
	case code := <-finished:
		if code != 4 {
			t.Fatalf("canceled discovery exit=%d output=%s stderr=%s", code, &stdout, &stderr)
		}
	case <-timeout.C:
		t.Fatal("canceled discovery did not return")
	}

	var reply response

	err = json.Unmarshal(stdout.Bytes(), &reply)
	if err != nil || reply.OK || len(reply.Diagnostics) != 1 {
		t.Fatalf("invalid canceled discovery response: %v: %s", err, &stdout)
	}
}

func waitForBlockingGo(t *testing.T, ready string, finished <-chan int, stdout, stderr *bytes.Buffer) {
	t.Helper()

	const (
		pollInterval     = 10 * time.Millisecond
		readinessTimeout = 10 * time.Second
	)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	timeout := time.NewTimer(readinessTimeout)
	defer timeout.Stop()

	for {
		_, err := os.Stat(ready)
		if err == nil {
			return
		}

		if !os.IsNotExist(err) {
			t.Fatalf("inspect blocking Go command: %v", err)
		}

		select {
		case code := <-finished:
			t.Fatalf("discovery exited before cancellation: %d: %s %s", code, stdout, stderr)
		case <-ticker.C:
		case <-timeout.C:
			t.Fatal("discovery did not start the Go command")
		}
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("OTELPLAN_TEST_VERBOSE_BACKEND") == "1" {
		_, _ = fmt.Fprintln(os.Stderr, "private-backend-stderr-token")

		os.Exit(23)
	}

	if os.Getenv("OTELPLAN_TEST_BLOCKING_GO") == "1" {
		err := os.WriteFile("ready", []byte("ready"), 0o600)
		if err != nil {
			os.Exit(1)
		}

		time.Sleep(30 * time.Second)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestVerboseBackendFailureRedactsSubprocessOutput(t *testing.T) {
	root, _ := cliFixture(t)
	backendDir := copyVerboseTestBackend(t)
	t.Setenv("PATH", backendDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OTELPLAN_TEST_VERBOSE_BACKEND", "1")

	for _, format := range []string{"text", "json"} {
		var stdout, stderr bytes.Buffer

		args := []string{"compile", "--root", root, "--offline", "--verbose", "--format=" + format}
		if exit := Run(t.Context(), args, &stdout, &stderr); exit != 7 {
			t.Fatalf("backend failure exit=%d: %s %s", exit, &stdout, &stderr)
		}

		if strings.Contains(stdout.String()+stderr.String(), "private-backend-stderr-token") ||
			!strings.Contains(stdout.String(), "subprocess exited with status 23") ||
			!strings.Contains(stdout.String(), "verify pinned backend") {
			t.Fatalf("unsafe or unhelpful verbose failure: %s %s", &stdout, &stderr)
		}
	}
}

func copyVerboseTestBackend(t *testing.T) string {
	t.Helper()

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	source := openIntegrationDirectory(t, filepath.Dir(executable))

	data, err := source.ReadFile(filepath.Base(executable))
	if err != nil {
		t.Fatal(err)
	}

	destination := openIntegrationDirectory(t, t.TempDir())

	name := "otelc"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	err = destination.WriteFile(name, data, 0o700)
	if err != nil {
		t.Fatal(err)
	}

	return destination.Name()
}

func TestSafeDiagnosticCausesOmitSensitiveValues(t *testing.T) {
	t.Parallel()

	pathError := &os.PathError{Op: "private-operation", Path: "/private-path", Err: syscall.EACCES}
	cause := fmt.Errorf("private-wrapper: %w", pathError)

	actual := strings.Join(safeDiagnosticCauses(cause), " ")
	if strings.Contains(actual, "private") || !strings.Contains(actual, "operation failed") {
		t.Fatalf("unsafe cause chain: %s", actual)
	}

	cycle := new(cyclicDiagnosticError)

	causes := safeDiagnosticCauses(cycle)
	if len(causes) != maximumDiagnosticCauses || causes[len(causes)-1] != "additional causes omitted" {
		t.Fatalf("cyclic chain was not bounded: %v", causes)
	}
}

type cyclicDiagnosticError struct{}

func TestInventoryOutputPreservesInternalBuildFingerprint(t *testing.T) {
	t.Parallel()

	root, _ := cliFixture(t)
	analysis := new(discovery.Options)
	analysis.Root, analysis.Offline = root, true

	code, err := discovery.LoadContext(t.Context(), *analysis)
	if err != nil {
		t.Fatal(err)
	}

	code.EffectiveBuild.CGOEnabled = "1"
	code.EffectiveBuild.CGOCFLAGS = "caller-private-build-value"

	before, err := lockfile.GraphDigest(code)
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer

	opts := new(options)
	opts.format = jsonFormat
	reply := new(response)
	reply.Data = code

	err = emit(&output, *opts, *reply)
	if err != nil {
		t.Fatal(err)
	}

	after, err := lockfile.GraphDigest(code)
	if err != nil || before != after || code.EffectiveBuild.CGOCFLAGS != "caller-private-build-value" ||
		bytes.Contains(output.Bytes(), []byte("caller-private-build-value")) {
		t.Fatal("inventory redaction exposed a value or changed internal build identity")
	}

	code.EffectiveBuild.CGOCFLAGS = "different-private-build-value"

	changed, err := lockfile.GraphDigest(code)
	if err != nil || changed == before {
		t.Fatal("free-form build values no longer influence the fingerprint")
	}
}

func TestVerboseBuildContextOmitsSensitiveConfiguration(t *testing.T) {
	t.Parallel()

	opts := new(options)
	opts.details = new(diagnosticDetails)
	build := new(model.BuildEnvironment)
	build.GOOS, build.GOARCH, build.ModuleMode = "linux", "amd64", "readonly"
	build.CGOCFLAGS, build.ModFile = "-DPRIVATE=private-token", "/private-path/go.mod"
	build.CC = "private-compiler-command"
	recordBuildContext(*opts, *build)

	encoded, err := json.Marshal(opts.details)
	if err != nil || bytes.Contains(encoded, []byte("private")) || !bytes.Contains(encoded, []byte("linux")) {
		t.Fatalf("unsafe or missing build context: %s (%v)", encoded, err)
	}
}

func (failure *cyclicDiagnosticError) Error() string {
	return "private-error-token"
}

func (failure *cyclicDiagnosticError) Unwrap() error {
	return failure
}

func TestInspectRejectsUnsafeCaptureWithoutPrintingConstant(t *testing.T) {
	root, files := cliFixture(t)

	contents := files["otelplan.yaml"] + "  attributes:\n  - key: password\n    from:\n      constant: do-not-print-this-secret\n"

	err := os.WriteFile(filepath.Join(root, "otelplan.yaml"), []byte(contents), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	var out, errout bytes.Buffer
	if code := Run(t.Context(), []string{"inspect", "--root", root, "--format=json"}, &out, &errout); code != 5 {
		t.Fatalf("unsafe inspection exit=%d output=%s", code, &out)
	}

	if strings.Contains(out.String(), "do-not-print-this-secret") {
		t.Fatal("unsafe constant printed")
	}
}
