package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
)

func TestInitDryRunJSONPreviewsStarterPolicyWithoutWriting(t *testing.T) {
	t.Parallel()

	root, directory, _ := callGraphFixture(t)
	path := filepath.Join(root, "otelplan.yaml")
	before := testenv.SnapshotFiles(t, root)

	var stdout, stderr bytes.Buffer

	args := []string{"init", "--root", root, "--offline", "--non-interactive", "--dry-run", "--format=json"}
	if exit := cli.Run(t.Context(), args, &stdout, &stderr); exit != 0 {
		t.Fatalf("dry-run init exit=%d: %s %s", exit, &stdout, &stderr)
	}

	var reply struct {
		OK   bool `json:"ok"`
		Data struct {
			Path   string `json:"path"`
			DryRun bool   `json:"dryRun"`
			Policy string `json:"policy"`
		} `json:"data"`
	}

	err := json.Unmarshal(stdout.Bytes(), &reply)
	if err != nil {
		t.Fatal(err)
	}

	if !reply.OK || reply.Data.Path != path || !reply.Data.DryRun || reply.Data.Policy == "" {
		t.Fatalf("invalid dry-run response: %s", stdout.Bytes())
	}

	_, err = directory.Stat("otelplan.yaml")
	if err == nil {
		t.Fatal("dry-run init created a policy")
	}

	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))
}

func TestInitDryRunPolicyMatchesSubsequentInit(t *testing.T) {
	t.Parallel()

	root, directory, _ := callGraphFixture(t)
	args := make([]string, 0, 7)
	args = append(args, "init", "--root", root, "--offline", "--non-interactive", "--format=json")
	previewArgs := append(append([]string{}, args...), "--dry-run")
	preview := runInitJSON(t, previewArgs)

	if preview.DryRun != true || preview.Policy == "" {
		t.Fatalf("missing dry-run preview: %+v", preview)
	}

	actual := runInitJSON(t, append(args, "--force"))
	if actual.HasDryRun || actual.HasPolicy || actual.DryRun || actual.Policy != "" {
		t.Fatalf("normal init unexpectedly included preview fields: %+v", actual)
	}

	generated, err := directory.ReadFile("otelplan.yaml")
	if err != nil {
		t.Fatal(err)
	}

	if string(generated) != preview.Policy {
		t.Fatal("dry-run policy differs from subsequent init output")
	}
}

func TestInitDryRunForcePreservesExistingDestination(t *testing.T) {
	t.Parallel()

	root, directory, _ := callGraphFixture(t)
	original := []byte("existing policy\n")

	err := directory.WriteFile("otelplan.yaml", original, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	args := make([]string, 0, 8)

	args = append(args, "init", "--root", root, "--offline", "--non-interactive", "--format=json", "--dry-run")
	if exit := runInit(t, args); exit != 1 {
		t.Fatalf("dry-run without force exit=%d, want 1", exit)
	}

	preview := runInitJSON(t, append(args, "--force"))
	if !preview.DryRun || preview.Policy == "" {
		t.Fatalf("invalid forced preview: %+v", preview)
	}

	after, err := directory.ReadFile("otelplan.yaml")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(after, original) {
		t.Fatal("forced dry-run changed the existing policy")
	}
}

func TestInitDryRunPreservesCacheAndMatchesUncachedPreview(t *testing.T) {
	t.Parallel()

	root, _, _ := callGraphFixture(t)
	cachePath := filepath.Join(root, "analysis-cache")
	before := testenv.SnapshotFiles(t, root)
	uncachedArgs := []string{"init", "--root", root, "--offline", "--non-interactive", "--format=json",
		"--dry-run", "--cache-dir", cachePath}
	uncached := runInitJSON(t, uncachedArgs)

	_, err := os.Stat(cachePath)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uncached dry-run created cache: %v", err)
	}

	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))

	populateArgs := []string{"init", "--root", root, "--offline", "--non-interactive", "--format=json",
		"--cache-dir", cachePath, "--force"}
	if exit := runInit(t, populateArgs); exit != 0 {
		t.Fatalf("cache population init exit=%d", exit)
	}

	cache := openCacheRoot(t, cachePath)

	if files := cacheFiles(t, cache); len(files) == 0 {
		t.Fatal("cache population init did not create cache files")
	}

	before = testenv.SnapshotFiles(t, root)
	warmArgs := []string{"init", "--root", root, "--offline", "--non-interactive", "--format=json",
		"--dry-run", "--cache-dir", cachePath, "--force"}
	warm := runInitJSON(t, warmArgs)

	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))

	if warm.Policy != uncached.Policy {
		t.Fatal("cached dry-run policy differs from uncached preview")
	}
}

func TestInitDryRunDestinationValidation(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		output string
		setup  func(string, *os.Root) error
	}{
		{name: "invalid extension", output: "starter.txt", setup: nil},
		{name: "missing parent", output: filepath.Join("missing", "starter.yaml"), setup: nil},
		{name: "directory", output: "starter.yaml", setup: func(_ string, directory *os.Root) error {
			return directory.Mkdir("starter.yaml", 0o700)
		}},
		{name: "symlink", output: "link.yaml", setup: func(root string, _ *os.Root) error {
			return os.Symlink(filepath.Join(root, "target.yaml"), filepath.Join(root, "link.yaml"))
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root, directory, _ := callGraphFixture(t)
			if testCase.setup != nil {
				err := testCase.setup(root, directory)
				if err != nil {
					t.Fatal(err)
				}
			}

			args := []string{"init", "--root", root, "--offline", "--non-interactive", "--format=json",
				"--dry-run", "--output", testCase.output}
			wantExit := 1

			if testCase.name == "invalid extension" {
				wantExit = 2
			}

			if exit := runInit(t, args); exit != wantExit {
				t.Fatalf("exit=%d, want %d", exit, wantExit)
			}
		})
	}
}

func TestInitDryRunInteractiveTextDoesNotWrite(t *testing.T) {
	t.Parallel()

	root, _, _ := callGraphFixture(t)

	var stdout, stderr bytes.Buffer

	args := []string{"init", "--root", root, "--offline", "--interactive", "--dry-run"}
	if exit := cli.RunWithInput(t.Context(), args, strings.NewReader("yes\nno\n"), &stdout, &stderr); exit != 0 {
		t.Fatalf("interactive dry-run exit=%d: %s %s", exit, &stdout, &stderr)
	}

	output := stdout.String() + stderr.String()
	if !strings.Contains(output, "would write") ||
		!strings.Contains(output, "apiVersion:") || strings.Contains(output, "wrote") {
		t.Fatalf("interactive dry-run did not describe preview: %s", output)
	}

	_, err := os.Stat(filepath.Join(root, "otelplan.yaml"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatal("interactive dry-run wrote a policy")
	}
}

func TestInitDryRunInteractiveJSONIsRejected(t *testing.T) {
	t.Parallel()

	root, _, _ := callGraphFixture(t)

	var stdout, stderr bytes.Buffer

	args := []string{"init", "--root", root, "--offline", "--interactive", "--dry-run", "--format=json"}
	if exit := cli.RunWithInput(t.Context(), args, strings.NewReader("yes\n"), &stdout, &stderr); exit != 2 {
		t.Fatalf("interactive JSON dry-run exit=%d: %s %s", exit, &stdout, &stderr)
	}
}

func TestInitDryRunCancellationAndNoCandidatesPreserveProject(t *testing.T) {
	t.Parallel()

	root, _, _ := callGraphFixture(t)
	before := testenv.SnapshotFiles(t, root)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var stdout, stderr bytes.Buffer

	args := []string{"init", "--root", root, "--offline", "--dry-run", "--format=json"}
	if exit := cli.Run(ctx, args, &stdout, &stderr); exit == 0 {
		t.Fatalf("canceled dry-run unexpectedly succeeded: %s", stdout.Bytes())
	}

	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))

	noContext := t.TempDir()

	err := os.WriteFile(filepath.Join(noContext, "go.mod"), []byte("module example.com/no-context\n\ngo 1.27\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(noContext, "app.go"), []byte("package app\nfunc Run() {}\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	before = testenv.SnapshotFiles(t, noContext)

	stdout.Reset()
	stderr.Reset()

	args = []string{"init", "--root", noContext, "--offline", "--dry-run", "--format=json"}
	if exit := cli.Run(t.Context(), args, &stdout, &stderr); exit != 5 {
		t.Fatalf("no-candidate dry-run exit=%d: %s %s", exit, stdout.Bytes(), stderr.Bytes())
	}

	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, noContext))
}

type initJSONPreview struct {
	DryRun    bool
	Policy    string
	HasDryRun bool
	HasPolicy bool
}

func runInitJSON(t *testing.T, args []string) initJSONPreview {
	t.Helper()

	var stdout, stderr bytes.Buffer

	if exit := cli.Run(t.Context(), args, &stdout, &stderr); exit != 0 {
		t.Fatalf("init exit=%d: %s %s", exit, stdout.Bytes(), stderr.Bytes())
	}

	var reply struct {
		OK   bool `json:"ok"`
		Data struct {
			DryRun json.RawMessage `json:"dryRun"`
			Policy json.RawMessage `json:"policy"`
		} `json:"data"`
	}

	err := json.Unmarshal(stdout.Bytes(), &reply)
	if err != nil {
		t.Fatal(err)
	}

	if !reply.OK {
		t.Fatalf("init response failed: %s", stdout.Bytes())
	}

	var dryRun bool

	if len(reply.Data.DryRun) > 0 {
		err = json.Unmarshal(reply.Data.DryRun, &dryRun)
		if err != nil {
			t.Fatal(err)
		}
	}

	var policy string

	if len(reply.Data.Policy) > 0 {
		err = json.Unmarshal(reply.Data.Policy, &policy)
		if err != nil {
			t.Fatal(err)
		}
	}

	return initJSONPreview{
		DryRun:    dryRun,
		Policy:    policy,
		HasDryRun: len(reply.Data.DryRun) > 0,
		HasPolicy: len(reply.Data.Policy) > 0,
	}
}

func runInit(t *testing.T, args []string) int {
	t.Helper()

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), args, &stdout, &stderr)

	return exit
}
