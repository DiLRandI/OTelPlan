package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestLockedBuildRejectsUnverifiedResolution(t *testing.T) {
	backend := lockedBuildBackend(t)
	t.Setenv("PATH", backend.Name()+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GOFLAGS", "")

	for _, state := range []string{
		"missing", "malformed", "policy drift", "signature drift", "build tags", "module mode",
	} {
		for _, format := range []string{"text", "json"} {
			for _, dryRun := range []bool{false, true} {
				checkLockedBuildFailure(t, state, format, dryRun)
			}
		}
	}

	_, err := backend.Lstat("otelc.invoked")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock failures invoked the backend: %v", err)
	}
}

func lockedBuildBackend(t *testing.T) *os.Root {
	t.Helper()

	backend := openLockTestRoot(t, t.TempDir())
	source := "#!/bin/sh\nprintf invoked > \"$0.invoked\"\nexit 1\n"
	requireTestSuccess(t, backend.WriteFile("otelc", []byte(source), 0o600), "write backend probe")
	requireTestSuccess(t, backend.Chmod("otelc", 0o700), "make backend probe executable")

	return backend
}

func checkLockedBuildFailure(t *testing.T, state, format string, dryRun bool) {
	t.Helper()

	root, directory, files := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n", executionPolicy)
	if state != "missing" {
		invoke(t, root, 0, "lock")
	}

	prepareLockedBuildFailure(t, directory, files, state)
	before := testenv.SnapshotFiles(t, root)

	args := []string{"build", "--root", root, "--offline", "--format=" + format, "--locked"}
	if dryRun {
		args = append(args, "--dry-run")
	}

	args = append(args, "--", "-o", "uncreated/probe")

	switch state {
	case "build tags":
		args = append(args, "-tags=changed")
	case "module mode":
		args = append(args, "-mod=mod")
	}

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), args, &stdout, &stderr)
	if exit != 6 || stderr.Len() != 0 {
		t.Fatalf("%s/%s/dry=%t: exit=%d stdout=%s stderr=%s", state, format, dryRun, exit, &stdout, &stderr)
	}

	checkLockedBuildDiagnostic(t, stdout.Bytes(), format)
	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))
}

func prepareLockedBuildFailure(t *testing.T, directory *os.Root, files map[string]string, state string) {
	t.Helper()

	switch state {
	case "malformed":
		writeLockTestFile(t, directory, "otelplan.lock", "malformed")
	case "policy drift":
		writeLockTestFile(t, directory, "otelplan.yaml", executionPolicy+"  span: {name: changed}\n")
	case "signature drift":
		changed := strings.Replace(files["app.go"], "context.Context)", "context.Context, value int)", 1)
		writeLockTestFile(t, directory, "app.go", changed)
	}
}

func checkLockedBuildDiagnostic(t *testing.T, output []byte, format string) {
	t.Helper()

	if format == "text" {
		if !strings.HasPrefix(string(output), "error OTP6001:") {
			t.Fatalf("missing stale-lock text diagnostic: %s", output)
		}

		return
	}

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil || reply.OK || reply.Command != "build" || reply.APIVersion != cli.APIVersion {
		t.Fatalf("invalid locked-build failure: %v, %s", err, output)
	}

	checkPolicyDiagnosticCodes(t, reply, []model.Code{model.CodeStaleLockfile})

	if reply.Diagnostics[0].Severity != model.SeverityError || reply.Diagnostics[0].Message == "" {
		t.Fatalf("missing locked-build error explanation: %+v", reply.Diagnostics)
	}
}

func TestLockedBuildAllowsFreshResolutionWithoutRefreshingIt(t *testing.T) {
	backend := lockedBuildBackend(t)
	t.Setenv("PATH", backend.Name()+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GOFLAGS", "")

	for _, state := range []string{"current", "line movement", "build metadata"} {
		checkFreshLockedBuild(t, state)
	}

	_, err := backend.Lstat("otelc.invoked")
	requireTestSuccess(t, err, "observe backend invocation after lock check")
}

func checkFreshLockedBuild(t *testing.T, state string) {
	t.Helper()

	root, directory, files := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n", executionPolicy)
	invoke(t, root, 0, "lock")

	switch state {
	case "line movement":
		writeLockTestFile(t, directory, "app.go", "\n\n"+files["app.go"])
	case "build metadata":
		addLockedBuildMetadata(t, directory)
	}

	before := testenv.SnapshotFiles(t, root)
	reply := invoke(t, root, 7, "build", "--locked")
	checkPolicyDiagnosticCodes(t, reply, []model.Code{model.CodeBackendVersionMismatch})
	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, directory.Name()))
}

func addLockedBuildMetadata(t *testing.T, directory *os.Root) {
	t.Helper()

	contents := readLockTestFile(t, directory, "otelplan.lock")
	locked, err := lockfile.Parse(contents)
	requireTestSuccess(t, err, "parse current resolution lock")

	locked.Backend.Digest = "sha256:" + strings.Repeat("0", 64)
	locked.Artifacts = []model.ArtifactFile{{Path: "generated/hooks.go", Digest: locked.Backend.Digest}}
	contents, err = lockfile.Marshal(locked)
	requireTestSuccess(t, err, "encode independently owned build metadata")
	writeLockTestFile(t, directory, "otelplan.lock", string(contents))
}

func TestUnlockedBuildStillPermitsMissingResolutionLock(t *testing.T) {
	backend := lockedBuildBackend(t)
	t.Setenv("PATH", backend.Name()+string(os.PathListSeparator)+os.Getenv("PATH"))
	root, _, _ := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(ctx context.Context) error { return nil }\n", executionPolicy)
	reply := invoke(t, root, 7, "build", "--locked=false")
	checkPolicyDiagnosticCodes(t, reply, []model.Code{model.CodeBackendVersionMismatch})

	_, err := backend.Lstat("otelc.invoked")
	requireTestSuccess(t, err, "observe explicitly unlocked backend invocation")
}

func TestLockedBuildFlagApplicability(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"--locked", "--locked=false"} {
		for _, format := range []string{"text", "json"} {
			checkCommandOptionFailure(t, []string{"inspect", value}, format, "--locked is supported by build")
		}
	}

	reply := runGlobalArgumentsJSON(t, []string{"build", "--locked=private-value", "--format=json"}, 2)
	if reply.Command != "build" || reply.Diagnostics[0].Message != "invalid flag value or syntax" {
		t.Fatalf("invalid locked flag failure: %+v", reply)
	}
}
