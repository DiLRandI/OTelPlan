package otelc_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
)

func TestExecutableVersion(t *testing.T) {
	executable := buildVersionProbeFixture(t)

	tests := []struct {
		name     string
		behavior string
		want     string
		wantErr  bool
		private  string
	}{
		{name: "supported", behavior: "supported", want: "v1.1.0", wantErr: false, private: ""},
		{name: "newer", behavior: "newer", want: "v1.2.0", wantErr: false, private: ""},
		{name: "build metadata", behavior: "metadata", want: "v1.1.0", wantErr: false, private: ""},
		{name: "trailing context", behavior: "trailing", want: "v1.1.0", wantErr: false, private: ""},
		{name: "malformed", behavior: "malformed", want: "", wantErr: true, private: "private-malformed-output"},
		{name: "wrong label", behavior: "wrong-label", want: "", wantErr: true, private: "private-wrong-label"},
		{name: "invalid version", behavior: "invalid", want: "", wantErr: true, private: "private-invalid-version"},
		{name: "abbreviated version", behavior: "abbreviated", want: "", wantErr: true, private: ""},
		{name: "oversized stdout", behavior: "oversized", want: "", wantErr: true, private: "private-oversized-output"},
		{name: "failed exit", behavior: "failed", want: "", wantErr: true, private: "private-failure"},
		{name: "stderr is discarded", behavior: "stderr", want: "v1.1.0", wantErr: false, private: ""},
		{name: "stderr failure is redacted", behavior: "stderr-failed", want: "", wantErr: true, private: "private-stderr"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("OTELPLAN_TEST_VERSION_MODE", testCase.behavior)

			version, err := otelc.ExecutableVersion(t.Context(), executable)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("version probe unexpectedly succeeded")
				}

				if testCase.private != "" && strings.Contains(err.Error(), testCase.private) {
					t.Fatalf("version probe error leaked private output: %v", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("version probe failed: %v", err)
			}

			if version != testCase.want {
				t.Fatalf("version = %q, want %q", version, testCase.want)
			}
		})
	}
}

func TestExecutableVersionMissingExecutable(t *testing.T) {
	t.Parallel()

	version, err := otelc.ExecutableVersion(t.Context(), filepath.Join(t.TempDir(), "missing-otelc"))
	if err == nil {
		t.Fatal("missing executable unexpectedly succeeded")
	}

	if version != "" {
		t.Fatalf("version = %q, want empty version", version)
	}
}

func TestExecutableVersionCancellation(t *testing.T) {
	executable := buildVersionProbeFixture(t)
	ready := filepath.Join(t.TempDir(), "ready")
	t.Setenv("OTELPLAN_TEST_VERSION_MODE", "hang\n"+ready)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	result := make(chan executableVersionResult, 1)

	go func() {
		version, err := otelc.ExecutableVersion(ctx, executable)
		result <- executableVersionResult{version: version, err: err}
	}()

	waitForReady(t, ready)
	cancel()

	select {
	case probe := <-result:
		if !errors.Is(probe.err, context.Canceled) {
			t.Fatalf("version probe error = %v, want context cancellation", probe.err)
		}

		if probe.version != "" {
			t.Fatalf("version = %q, want empty version", probe.version)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("version probe did not stop after context cancellation")
	}
}

type executableVersionResult struct {
	version string
	err     error
}

func waitForReady(t *testing.T, path string) {
	t.Helper()

	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatalf("open readiness directory: %v", err)
	}
	defer func() {
		closeErr := root.Close()
		if closeErr != nil {
			t.Errorf("close readiness directory: %v", closeErr)
		}
	}()

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()

	for {
		_, err = root.Stat(filepath.Base(path))
		if err == nil {
			return
		}

		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("version probe did not become ready: %v", err)
		}
	}
}

func buildVersionProbeFixture(t *testing.T) string {
	t.Helper()

	directory := t.TempDir()
	source := filepath.Join(directory, "main.go")
	writeTestFile(t, source, versionProbeSource)

	command := exec.CommandContext(t.Context(), "go", "build", "-o", ".", "main.go")
	command.Dir = directory

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build version probe: %v\n%s", err, output)
	}

	return versionProbePath(directory, "main")
}

func versionProbePath(directory, name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	return filepath.Join(directory, name)
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()

	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatalf("open test fixture directory: %v", err)
	}

	err = root.WriteFile(filepath.Base(path), []byte(contents), 0o600)
	closeErr := root.Close()

	if err != nil {
		t.Fatalf("write test fixture %s: %v", path, err)
	}

	if closeErr != nil {
		t.Fatalf("close test fixture directory: %v", closeErr)
	}
}

const versionProbeSource = `package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "version" {
		fmt.Fprintln(os.Stderr, "private-arguments")
		os.Exit(4)
	}

	behavior := strings.SplitN(os.Getenv("OTELPLAN_TEST_VERSION_MODE"), "\n", 2)
	switch behavior[0] {
	case "supported":
		fmt.Println("otelc version v1.1.0")
	case "newer":
		fmt.Println("otelc version v1.2.0")
	case "metadata":
		fmt.Println("otelc version v1.1.0+private-build")
	case "trailing":
		fmt.Println("otelc version v1.1.0 built with Go")
	case "malformed":
		fmt.Println("private-malformed-output")
	case "wrong-label":
		fmt.Println("private-label version v1.1.0 private-wrong-label")
	case "invalid":
		fmt.Println("otelc version private-invalid-version")
	case "abbreviated":
		fmt.Println("otelc version v1.1")
	case "oversized":
		fmt.Print(strings.Repeat("private-oversized-output", 300))
	case "failed":
		fmt.Fprintln(os.Stderr, "private-failure")
		os.Exit(1)
	case "stderr":
		fmt.Fprintln(os.Stderr, "private-stderr")
		fmt.Println("otelc version v1.1.0")
	case "stderr-failed":
		fmt.Fprintln(os.Stderr, "private-stderr")
		os.Exit(1)
	case "hang":
		if len(behavior) != 2 {
			os.Exit(5)
		}
		err := os.WriteFile(behavior[1], []byte("ready"), 0o600)
		if err != nil {
			os.Exit(6)
		}
		time.Sleep(time.Hour)
	default:
		os.Exit(3)
	}
}
`
