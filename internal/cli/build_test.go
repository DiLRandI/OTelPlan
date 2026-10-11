package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/internal/lockfile"
)

func TestBuildCLIWithPinnedBackend(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required")
	}

	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	prepareOfflineIntegration(t)
	root := pinnedBuildFixture(t)
	original := snapshotPinnedBuildFixture(t, root)
	cliPath := buildPinnedCLI(t)
	probe := runPinnedBuild(t, cliPath, root)
	assertPinnedArtifact(t, probe, filepath.Join(root, "bin", "probe"))
	assertPinnedTrace(t, probe.Path)

	for _, existing := range []bool{false, true} {
		assertPinnedDirectoryBuild(t, cliPath, root, existing)
	}

	assertPinnedSourceGuard(t, root)
	assertPinnedBuildFixtureUnchanged(t, root, original)
}

func pinnedBuildFixture(t *testing.T) string {
	t.Helper()

	fixture, err := filepath.Abs("../backend/otelc/testdata/accessors")
	if err != nil {
		t.Fatal(err)
	}

	root, err := compiler.CopySourceTree(t.Context(), fixture, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	directory := openIntegrationDirectory(t, root)
	policy := []byte("apiVersion: otelplan.io/v1alpha1\n" +
		"kind: InstrumentationPlan\n" +
		"backend: {name: otelc, version: v1.1.0}\n" +
		"project: {packages: [./ops]}\nrules:\n- id: operation\n" +
		"  match:\n    methods: [Handle]\n")

	err = directory.WriteFile("otelplan.yaml", policy, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = directory.MkdirAll("cmd/second", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	mainSource, err := directory.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}

	err = directory.WriteFile("cmd/second/main.go", mainSource, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return root
}

func snapshotPinnedBuildFixture(t *testing.T, root string) map[string][]byte {
	t.Helper()

	directory := openIntegrationDirectory(t, root)
	files := make(map[string][]byte)

	err := fs.WalkDir(directory.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("snapshot build fixture %s: %w", name, walkErr)
		}

		if entry.IsDir() {
			return nil
		}

		data, readErr := directory.ReadFile(name)
		if readErr != nil {
			return fmt.Errorf("snapshot build fixture %s: %w", name, readErr)
		}

		files[name] = data

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return files
}

func buildPinnedCLI(t *testing.T) string {
	t.Helper()

	cliPath := filepath.Join(t.TempDir(), "otelplan")
	command := exec.CommandContext(t.Context(), "go", "build", "-o", cliPath, "../../cmd/otelplan")

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build CLI: %v %s", err, output)
	}

	return cliPath
}

type pinnedBuildReply struct {
	OK   bool         `json:"ok"`
	Data buildSummary `json:"data"`
}

func runPinnedBuild(t *testing.T, cliPath, root string) buildSummary {
	t.Helper()

	args := []string{"build", "--root", root, "--format=json", "--offline", "--",
		"-race", "-trimpath", "-buildvcs=false", "-o", "bin/probe", "."}
	command := exec.CommandContext(t.Context(), cliPath, args...)

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build command: %v %s", err, output)
	}

	var reply pinnedBuildReply

	err = json.Unmarshal(output, &reply)
	if err != nil || !reply.OK || reply.Data.Digest == "" {
		t.Fatalf("bad build JSON: %s", output)
	}

	return reply.Data
}

func assertPinnedArtifact(t *testing.T, artifact buildSummary, expectedPath string) {
	t.Helper()

	if artifact.Path != expectedPath {
		t.Fatalf("build path=%s, want %s", artifact.Path, expectedPath)
	}

	directory := openIntegrationDirectory(t, filepath.Dir(artifact.Path))

	data, err := directory.ReadFile(filepath.Base(artifact.Path))
	if err != nil {
		t.Fatal(err)
	}

	if artifact.Digest != lockfile.Digest(data) {
		t.Fatalf("build digest=%s does not match artifact", artifact.Digest)
	}
}

type pinnedTraceSpan struct {
	Name string `json:"Name"`
}

type pinnedTraceReply struct {
	Spans []pinnedTraceSpan `json:"Spans"`
}

func assertPinnedTrace(t *testing.T, path string) {
	t.Helper()

	command := exec.CommandContext(t.Context(), path)

	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}

	var trace pinnedTraceReply

	err = json.Unmarshal(output, &trace)
	if err != nil || len(trace.Spans) != 5 {
		t.Fatalf("missing instrumented spans: %s", output)
	}
}

type pinnedDirectoryReply struct {
	OK   bool `json:"ok"`
	Data struct {
		Files []buildFile `json:"files"`
	} `json:"data"`
}

func assertPinnedDirectoryBuild(t *testing.T, cliPath, root string, existing bool) {
	t.Helper()

	destination, argument, directory := preparePinnedDirectoryOutput(t, existing)
	files := runPinnedDirectoryBuild(t, cliPath, root, argument)
	assertPinnedDirectoryFiles(t, files, destination)

	if existing {
		assertPinnedDirectoryPreflightFailure(t, root, destination, argument, cliPath, files)

		keep, err := directory.ReadFile("binaries/keep")
		if err != nil || string(keep) != "keep" {
			t.Fatal("changed unrelated output directory file")
		}
	}
}

func preparePinnedDirectoryOutput(t *testing.T, existing bool) (string, string, *os.Root) {
	t.Helper()

	parentPath := t.TempDir()
	directory := openIntegrationDirectory(t, parentPath)
	destination := filepath.Join(parentPath, "binaries")

	argument := destination + string(os.PathSeparator)
	if !existing {
		return destination, argument, directory
	}

	err := directory.MkdirAll("binaries", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	err = directory.WriteFile("binaries/keep", []byte("keep"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return destination, destination, directory
}

func runPinnedDirectoryBuild(t *testing.T, cliPath, root, argument string) []buildFile {
	t.Helper()

	args := []string{"build", "--root", root, "--format=json", "--offline", "--verbose", "--",
		"-buildvcs=false", "-o", argument, ".", "./cmd/second"}
	command := exec.CommandContext(t.Context(), cliPath, args...)

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("directory build: %v %s", err, output)
	}

	var reply pinnedDirectoryReply

	err = json.Unmarshal(output, &reply)
	if err != nil || !reply.OK || len(reply.Data.Files) != 2 {
		t.Fatalf("invalid directory response: %s", output)
	}

	return reply.Data.Files
}

func assertPinnedDirectoryFiles(t *testing.T, files []buildFile, destination string) {
	t.Helper()

	for _, file := range files {
		if filepath.Dir(file.Path) != destination || file.Digest == "" {
			t.Fatalf("invalid published file: %+v", file)
		}

		var artifact buildSummary

		artifact.Path, artifact.Digest = file.Path, file.Digest
		assertPinnedArtifact(t, artifact, file.Path)
		assertPinnedTrace(t, file.Path)
	}
}

func assertPinnedDirectoryPreflightFailure(t *testing.T, root, destination, argument, cliPath string,
	files []buildFile) {
	t.Helper()

	directory := openIntegrationDirectory(t, destination)
	first := preparePinnedPreflight(t, directory, files)
	command := exec.CommandContext(t.Context(), cliPath, "build", "--root", root, "--format=json", "--offline",
		"--", "-buildvcs=false", "-o", argument, ".", "./cmd/second")
	output, err := command.CombinedOutput()

	var exitErr *exec.ExitError
	if err == nil || !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("invalid destination unexpectedly succeeded: %s", output)
	}

	var reply response

	decodeErr := json.Unmarshal(output, &reply)
	if decodeErr != nil || reply.OK {
		t.Fatalf("invalid destination response: %s", output)
	}

	data, readErr := directory.ReadFile(first)
	if readErr != nil || string(data) != "previous" {
		t.Fatal("invalid destination partially replaced output")
	}
}

func preparePinnedPreflight(t *testing.T, directory *os.Root, files []buildFile) string {
	t.Helper()

	first, last := filepath.Base(files[0].Path), filepath.Base(files[1].Path)

	err := directory.WriteFile("previous", []byte("previous"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = directory.Rename("previous", first)
	if err != nil {
		t.Fatal(err)
	}

	err = directory.Remove(last)
	if err != nil {
		t.Fatal(err)
	}

	err = directory.Mkdir(last, 0o700)
	if err != nil {
		t.Fatal(err)
	}

	return first
}

func assertPinnedSourceGuard(t *testing.T, root string) {
	t.Helper()

	directory := openIntegrationDirectory(t, root)

	before, err := directory.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer

	exit := Run(t.Context(), []string{"build", "--root", root, "--format=json", "--", "-o", "go.mod", "."},
		&stdout, &stderr)
	if exit != 2 {
		t.Fatalf("source output exit=%d: %s", exit, &stdout)
	}

	after, err := directory.ReadFile("go.mod")
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source guard changed go.mod")
	}
}

func assertPinnedBuildFixtureUnchanged(t *testing.T, root string, original map[string][]byte) {
	t.Helper()

	current := snapshotPinnedBuildFixture(t, root)
	if len(current) != len(original)+1 {
		t.Fatalf("build changed source file set: got %d files, want %d plus bin/probe", len(current), len(original))
	}

	for name, want := range original {
		got, exists := current[name]
		if !exists || !bytes.Equal(got, want) {
			t.Fatalf("changed source file %s", name)
		}
	}

	for name := range current {
		if _, exists := original[name]; !exists && name != "bin/probe" {
			t.Fatalf("build added unexpected source file %s", name)
		}
	}
}

func TestBuildCLIUsage(t *testing.T) {
	for _, args := range [][]string{{"build", "--", "-o"}, {"build", "--", "-overlay=secret"}, {"build", "--check"}, {"build", "--clean"}} {
		var out, errout bytes.Buffer

		args = append([]string{"--format=json"}, args...)
		if code := Run(t.Context(), args, &out, &errout); code != 2 {
			t.Fatalf("usage exit=%d: %s", code, &out)
		}

		var reply response

		err := json.Unmarshal(out.Bytes(), &reply)
		if err != nil || reply.OK {
			t.Fatalf("invalid JSON: %s", &out)
		}
	}
}

func TestBuildCLILibraryWithoutOutput(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required")
	}

	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	prepareOfflineIntegration(t)
	runLibraryBuildContract(t)
}

func runLibraryBuildContract(t *testing.T) {
	t.Helper()

	root, _ := cliFixture(t)
	original := snapshotPinnedBuildFixture(t, root)

	var stdout, stderr bytes.Buffer

	args := []string{"build", "--root", root, "--offline", "--format=json", "--", "-buildvcs=false", "."}

	exit := Run(t.Context(), args, &stdout, &stderr)
	if exit != 0 || stderr.Len() != 0 {
		t.Fatalf("library build exit=%d: stdout=%s stderr=%s", exit, &stdout, &stderr)
	}

	assertLibraryBuildReply(t, stdout.Bytes())

	assertLibraryFixtureUnchanged(t, root, original)
}

func assertLibraryBuildReply(t *testing.T, output []byte) {
	t.Helper()

	var reply libraryBuildReply

	err := json.Unmarshal(output, &reply)
	if err != nil {
		t.Fatalf("invalid library response: %v, %s", err, output)
	}

	assertLibraryBuildEnvelope(t, reply, output)

	if string(reply.Diagnostics) != "[]" || len(reply.Data) == 0 || string(reply.Data) == "null" {
		t.Fatalf("library build response omitted data: %s", output)
	}

	var summary buildSummary

	err = json.Unmarshal(reply.Data, &summary)
	if err != nil || len(summary.Path) != 0 || len(summary.Digest) != 0 || len(summary.Files) != 0 {
		t.Fatalf("library build unexpectedly published output: %s", output)
	}
}

func assertLibraryBuildEnvelope(t *testing.T, reply libraryBuildReply, output []byte) {
	t.Helper()

	if reply.APIVersion != APIVersion || reply.Command != "build" || !reply.OK {
		t.Fatalf("unexpected library output: %s", output)
	}
}

type libraryBuildReply struct {
	APIVersion  string          `json:"apiVersion"`
	Command     string          `json:"command"`
	OK          bool            `json:"ok"`
	Diagnostics json.RawMessage `json:"diagnostics"`
	Data        json.RawMessage `json:"data"`
}

func assertLibraryFixtureUnchanged(t *testing.T, root string, original map[string][]byte) {
	t.Helper()

	current := snapshotPinnedBuildFixture(t, root)
	if len(current) != len(original) {
		t.Fatalf("library build changed file set: got %d files, want %d", len(current), len(original))
	}

	for name, want := range original {
		got, exists := current[name]
		if !exists || !bytes.Equal(got, want) {
			t.Fatalf("library build changed %s", name)
		}
	}
}

func TestBuildCLIFromWorkspaceRoot(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required")
	}

	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	prepareOfflineIntegration(t)
	t.Setenv("GOWORK", "")

	root := workspaceBuildFixture(t)

	original := snapshotPinnedBuildFixture(t, root)

	published := runWorkspaceBuild(t, root)
	if filepath.Dir(published.Path) != root {
		t.Fatalf("output not published in original working directory: %s", published.Path)
	}

	assertPinnedArtifact(t, published, published.Path)
	assertPinnedTrace(t, published.Path)
	assertWorkspaceFixtureUnchanged(t, root, original, published.Path)
	publishedBytes := readWorkspaceArtifact(t, published.Path)

	var stdout, stderr bytes.Buffer

	exit := Run(t.Context(), []string{"build", "--root", root, "--offline", "--format=json", "--", "-buildvcs=false"},
		&stdout, &stderr)
	if exit == 0 {
		t.Fatal("workspace build without target silently selected a module")
	}

	assertWorkspaceFailure(t, stdout.Bytes(), stderr.Bytes(), published.Path, publishedBytes)
	assertWorkspaceFixtureUnchanged(t, root, original, published.Path)
}

func workspaceBuildFixture(t *testing.T) string {
	t.Helper()

	fixture, err := filepath.Abs("../backend/otelc/testdata/accessors")
	if err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()

	copied, err := compiler.CopySourceTree(t.Context(), fixture, workspace)
	if err != nil {
		t.Fatal(err)
	}

	directory := openIntegrationDirectory(t, workspace)

	err = directory.Rename(filepath.Base(copied), "app")
	if err != nil {
		t.Fatal(err)
	}

	writeWorkspaceFile(t, directory, "go.work", "go 1.27.0\nuse ./app\n")
	writeWorkspaceFile(t, directory, "otelplan.yaml",
		"apiVersion: otelplan.io/v1alpha1\nkind: InstrumentationPlan\n"+
			"backend: {name: otelc, version: v1.1.0}\nproject: {packages: [./app/ops]}\n"+
			"rules:\n- id: operation\n  match:\n    methods: [Handle]\n")

	return workspace
}

func writeWorkspaceFile(t *testing.T, directory *os.Root, name, contents string) {
	t.Helper()

	err := directory.WriteFile(name, []byte(contents), 0o600)
	if err != nil {
		t.Fatal(err)
	}
}

func runWorkspaceBuild(t *testing.T, root string) buildSummary {
	t.Helper()

	var stdout, stderr bytes.Buffer

	args := []string{"build", "--root", root, "--offline", "--format=json", "--", "-buildvcs=false", "./app"}
	exit := Run(t.Context(), args,
		&stdout, &stderr)

	if exit != 0 || stderr.Len() != 0 {
		t.Fatalf("workspace build exit=%d: stdout=%s stderr=%s", exit, &stdout, &stderr)
	}

	var reply pinnedBuildReply

	err := json.Unmarshal(stdout.Bytes(), &reply)
	if err != nil || !reply.OK || reply.Data.Path == "" || reply.Data.Digest == "" {
		t.Fatalf("invalid build response: %s", &stdout)
	}

	return reply.Data
}

func assertWorkspaceFixtureUnchanged(t *testing.T, root string, original map[string][]byte, publishedPath string) {
	t.Helper()

	current := snapshotPinnedBuildFixture(t, root)

	publishedName, err := filepath.Rel(root, publishedPath)
	if err != nil {
		t.Fatal(err)
	}

	publishedName = filepath.ToSlash(publishedName)
	if len(current) != len(original)+1 {
		t.Fatalf("workspace changed file set: got %d files, want %d plus %s", len(current), len(original), publishedName)
	}

	for name, want := range original {
		got, exists := current[name]
		if !exists || !bytes.Equal(got, want) {
			t.Fatalf("changed workspace source file %s", name)
		}
	}

	if _, exists := current[publishedName]; !exists {
		t.Fatalf("published artifact missing from workspace snapshot: %s", publishedName)
	}

	for name := range current {
		if _, exists := original[name]; !exists && name != publishedName {
			t.Fatalf("unexpected workspace output %s", name)
		}
	}
}

func assertWorkspaceFailure(t *testing.T, output, stderr []byte, publishedPath string, before []byte) {
	t.Helper()

	var reply response

	err := json.Unmarshal(output, &reply)
	if err != nil || reply.OK || len(reply.Diagnostics) == 0 || len(stderr) != 0 {
		t.Fatalf("invalid no-target workspace response: stdout=%s stderr=%s", output, stderr)
	}

	after := readWorkspaceArtifact(t, publishedPath)
	if !bytes.Equal(before, after) {
		t.Fatal("failed workspace build changed published binary")
	}
}

func readWorkspaceArtifact(t *testing.T, publishedPath string) []byte {
	t.Helper()

	directory := openIntegrationDirectory(t, filepath.Dir(publishedPath))

	data, err := directory.ReadFile(filepath.Base(publishedPath))
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestBuildSummaryText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		path   string
		digest string
		files  []buildFile
		want   string
	}{
		{name: "empty", path: "", digest: "", files: nil, want: "build succeeded; no executable output\n"},
		{name: "executable", path: "app", digest: "one", files: nil, want: "built app one\n"},
		{name: "directory", files: []buildFile{
			{Path: "bin/first", Digest: "one"}, {Path: "bin/second", Digest: "two"},
		}, path: "", digest: "", want: "built bin/first one\nbuilt bin/second two\n"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var summary buildSummary

			summary.Path, summary.Digest = testCase.path, testCase.digest
			summary.Files = append(summary.Files, testCase.files...)

			var options options

			options.format = "text"

			var reply response

			reply.OK, reply.Data = true, summary

			var output bytes.Buffer

			err := emit(&output, options, reply)
			if err != nil || output.String() != testCase.want {
				t.Fatalf("output=%q, %v; want %q", output.String(), err, testCase.want)
			}
		})
	}
}

func TestVariadicBuiltinWithPinnedBackend(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required for pinned backend integration")
	}

	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	prepareOfflineIntegration(t)

	fixture, err := filepath.Abs("../backend/otelc/testdata/accessors")
	if err != nil {
		t.Fatal(err)
	}

	root, err := compiler.CopySourceTree(t.Context(), fixture, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	original := prepareVariadicFixture(t, root)

	var stdout, stderr bytes.Buffer

	args := []string{"build", "--root", root, "--offline", "--format=json", "--",
		"-trimpath", "-buildvcs=false", "-o", "bin/variadic", "."}
	if exit := Run(t.Context(), args, &stdout, &stderr); exit != 0 {
		t.Fatalf("variadic build exit=%d: %s %s", exit, &stdout, &stderr)
	}

	var reply struct {
		OK   bool         `json:"ok"`
		Data buildSummary `json:"data"`
	}

	err = json.Unmarshal(stdout.Bytes(), &reply)
	if err != nil || !reply.OK || reply.Data.Path == "" {
		t.Fatalf("invalid variadic build response: %s (%v)", &stdout, err)
	}

	command := exec.CommandContext(t.Context(), "./bin/variadic")
	command.Dir = root

	output, err := command.Output()
	if err != nil {
		t.Fatalf("run variadic build: %v", err)
	}

	checkVariadicTrace(t, output)
	checkVariadicSourceUnchanged(t, root, original)
}

func TestGenericRootSpansWithPinnedBackend(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required for pinned backend integration")
	}

	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	prepareOfflineIntegration(t)

	fixture, err := filepath.Abs("../backend/otelc/testdata/generics")
	if err != nil {
		t.Fatal(err)
	}

	root, err := compiler.CopySourceTree(t.Context(), fixture, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	invokeArchitecture(t, root, true, 0, "lock")
	original := snapshotArchitecture(t, root)
	generated := filepath.Join(t.TempDir(), "generated")
	invokeArchitecture(t, root, true, 0, "compile", "--output", generated)
	outputDir := t.TempDir()
	invokeArchitecture(t, root, true, 0, "build", "--", "-race", "-buildvcs=false",
		"-o", filepath.Join(outputDir, "generic-probe"), ".")

	command := exec.CommandContext(t.Context(), "./generic-probe")
	command.Dir = outputDir

	output, err := command.Output()
	if err != nil {
		t.Fatalf("run generic root fixture: %v", err)
	}

	checkGenericRootTrace(t, output)
	invokeArchitecture(t, root, true, 0, "lock", "--check")
	checkArchitectureImmutability(t, original, snapshotArchitecture(t, root))
	checkGenericContextRejection(t, root)
}

func TestGenericTypedCapturesWithPinnedBackend(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required for pinned backend integration")
	}

	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	prepareOfflineIntegration(t)

	fixture, err := filepath.Abs("../backend/otelc/testdata/generics")
	if err != nil {
		t.Fatal(err)
	}

	root, err := compiler.CopySourceTree(t.Context(), fixture, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	invokeArchitecture(t, root, true, 0, "lock", "--config", "capture.yaml")
	original := snapshotArchitecture(t, root)
	invokeArchitecture(t, root, true, 0, "compile", "--config", "capture.yaml",
		"--output", filepath.Join(t.TempDir(), "generated"))
	outputDir := t.TempDir()
	invokeArchitecture(t, root, true, 0, "build", "--config", "capture.yaml", "--", "-race", "-buildvcs=false",
		"-o", filepath.Join(outputDir, "capture-probe"), "./cmd/captures")

	command := exec.CommandContext(t.Context(), "./capture-probe")
	command.Dir = outputDir

	output, err := command.Output()
	if err != nil {
		t.Fatalf("run generic capture fixture: %v", err)
	}

	checkGenericCaptureTrace(t, output)
	invokeArchitecture(t, root, true, 0, "lock", "--config", "capture.yaml", "--check")
	checkArchitectureImmutability(t, original, snapshotArchitecture(t, root))
	checkGenericCaptureRejections(t, root)
	checkGenericReceiverInferenceFailure(t, root)
}

func checkGenericReceiverInferenceFailure(t *testing.T, root string) {
	t.Helper()

	directory := openIntegrationDirectory(t, root)
	policy := []byte(`apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
project: {packages: [./ops]}
defaults: {context: {mode: root}}
rules:
- id: inference
  match: {symbols: [example.com/genericprobe/ops.(Store).UnsupportedCapture]}
`)

	err := directory.WriteFile("inference.yaml", policy, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	original := snapshotArchitecture(t, root)
	output := openIntegrationDirectory(t, t.TempDir())
	invokeArchitecture(t, root, true, 8, "build", "--config", "inference.yaml", "--", "-buildvcs=false",
		"-o", filepath.Join(output.Name(), "blocked"), "./cmd/captures")

	_, err = output.Stat("blocked")
	if !os.IsNotExist(err) {
		t.Fatal("failed generic inference published a binary")
	}

	checkArchitectureImmutability(t, original, snapshotArchitecture(t, root))
}

type genericCapturePair struct {
	requestPresent bool
	enabled        bool
}

func checkGenericCaptureTrace(t *testing.T, output []byte) {
	t.Helper()

	if bytes.Contains(output, []byte("private-input-marker")) || bytes.Contains(output, []byte("private-request-marker")) {
		t.Fatal("generic capture emitted an unselected private value")
	}

	var reply struct {
		Spans []genericTraceSpan `json:"spans"`
	}

	err := json.Unmarshal(output, &reply)
	if err != nil {
		t.Fatalf("decode generic captures: %v", err)
	}

	const expectedCalls = 4
	if len(reply.Spans) != expectedCalls {
		t.Fatalf("unexpected generic capture span count: %s", output)
	}

	combinations := make(map[genericCapturePair]int, expectedCalls)
	for _, span := range reply.Spans {
		combinations[checkGenericCaptureSpan(t, span)]++
	}

	if len(combinations) != expectedCalls {
		t.Fatalf("direct inputs did not preserve method offsets or nil omission: %v", combinations)
	}
}

func checkGenericCaptureSpan(t *testing.T, span genericTraceSpan) genericCapturePair {
	t.Helper()

	if span.Name != "generic.capture" || span.Attributes["result.message"] != "approved-output" ||
		span.Attributes["result.empty"] != "" {
		t.Fatalf("incorrect generic result capture: %+v", span)
	}

	enabled, exists := span.Attributes["capture.enabled"].(bool)
	if !exists {
		t.Fatal("generic capture omitted a zero or true boolean value")
	}

	request, requestPresent := span.Attributes["request.id"]
	if requestPresent && request != "approved-id" {
		t.Fatal("generic capture changed a selected request field")
	}

	const baseCaptureCount = 3

	expectedCount := baseCaptureCount
	if requestPresent {
		expectedCount++
	}

	if len(span.Attributes) != expectedCount {
		t.Fatalf("generic capture included an overflow, non-finite, or unselected value: %+v", span)
	}

	return genericCapturePair{requestPresent: requestPresent, enabled: enabled}
}

func checkGenericCaptureRejections(t *testing.T, root string) {
	t.Helper()

	for _, rejection := range []struct {
		symbol, source, code, message string
		exit                          int
	}{
		{symbol: "Parametric", source: "req.ID", code: "OTP5001", message: "unbound type parameters", exit: 7},
		{symbol: "Capture", source: "req.Secret", code: "OTP4001", message: "secret", exit: 5},
	} {
		directory := openIntegrationDirectory(t, root)
		policy := "apiVersion: otelplan.io/v1alpha1\nkind: InstrumentationPlan\n" +
			"backend: {name: otelc, version: v1.1.0}\nproject: {packages: [./ops]}\n" +
			"defaults: {context: {mode: root}}\nrules:\n- id: rejected\n" +
			"  match: {symbols: [example.com/genericprobe/ops." + rejection.symbol + "]}\n" +
			"  attributes:\n  - key: request.value\n    from: {argument: " + rejection.source + "}\n"

		err := directory.WriteFile("rejected.yaml", []byte(policy), 0o600)
		if err != nil {
			t.Fatal(err)
		}

		original := snapshotArchitecture(t, root)
		reply := invokeArchitecture(t, root, true, rejection.exit, "validate", "--config", "rejected.yaml")
		found := false

		for _, diagnostic := range reply.Diagnostics {
			if string(diagnostic.Code) == rejection.code && strings.Contains(diagnostic.Message, rejection.message) {
				found = true
			}
		}

		if !found {
			t.Fatalf("missing generic capture rejection %s: %+v", rejection.code, reply.Diagnostics)
		}

		checkArchitectureImmutability(t, original, snapshotArchitecture(t, root))
	}
}

func checkGenericContextRejection(t *testing.T, root string) {
	t.Helper()

	directory := openIntegrationDirectory(t, root)
	policy := []byte(`apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
project: {packages: [./ops]}
defaults:
  context: {mode: root}
rules:
- id: contextual
  match: {symbols: [example.com/genericprobe/ops.Contextual]}
`)

	err := directory.WriteFile("otelplan.yaml", policy, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	original := snapshotArchitecture(t, root)
	invokeArchitecture(t, root, true, 7, "validate")
	checkArchitectureImmutability(t, original, snapshotArchitecture(t, root))
}

type genericTraceSpan struct {
	Name       string         `json:"name"`
	Parent     string         `json:"parent"`
	Error      bool           `json:"error"`
	Events     int            `json:"events"`
	Attributes map[string]any `json:"attributes"`
}

func checkGenericRootTrace(t *testing.T, output []byte) {
	t.Helper()

	if bytes.Contains(output, []byte("private-input-marker")) {
		t.Fatal("generic instrumentation captured an unselected argument")
	}

	var reply struct {
		Spans []genericTraceSpan `json:"spans"`
	}

	err := json.Unmarshal(output, &reply)
	if err != nil {
		t.Fatalf("decode generic trace: %v", err)
	}

	expected := map[string]int{
		"generic.function": 2, "generic.method": 2, "generic.panic": 1, "generic.batch": 1,
		"generic.pointer": 1,
	}
	counts := make(map[string]int, len(expected))
	failures := 0

	for _, span := range reply.Spans {
		checkGenericRootSpan(t, span)

		counts[span.Name]++
		if span.Error {
			failures++
		}
	}

	const expectedFailures = 2
	if failures != expectedFailures || len(counts) != len(expected) {
		t.Fatalf("unexpected generic spans or error recording: %s", output)
	}

	for name, count := range expected {
		if counts[name] != count {
			t.Fatalf("generic span %s ended %d times, want %d", name, counts[name], count)
		}
	}
}

func checkGenericRootSpan(t *testing.T, span genericTraceSpan) {
	t.Helper()

	if span.Parent != "0000000000000000" || len(span.Attributes) != 1 ||
		span.Attributes["operation.kind"] != strings.TrimPrefix(span.Name, "generic.") {
		t.Fatalf("generic span has a parent or unexpected captures: %+v", span)
	}

	expectedEvents := 0
	if span.Error {
		expectedEvents = 1
	}

	if span.Events != expectedEvents {
		t.Fatalf("generic span has incorrect error events: %+v", span)
	}
}

func prepareVariadicFixture(t *testing.T, root string) map[string][]byte {
	t.Helper()

	directory := openIntegrationDirectory(t, root)
	defer func() { _ = directory.Close() }()

	data, err := directory.ReadFile("ops/ops.go")
	if err != nil {
		t.Fatal(err)
	}

	data = append(data, []byte(`
func Variadic(ctx context.Context, values ...int) (int, error) {
	_, child := otel.Tracer("probe").Start(ctx, "variadic-child")
	child.End()
	total := 0
	for _, value := range values { total += value }
	return total, nil
}

func VariadicString(ctx context.Context, values ...string) error {
	_, child := otel.Tracer("probe").Start(ctx, "string-child")
	child.End()
	if len(values) != 2 { return errors.New("string arguments changed") }
	return nil
}

func VariadicAny(ctx context.Context, values ...any) error {
	_, child := otel.Tracer("probe").Start(ctx, "any-child")
	child.End()
	if len(values) != 2 { return errors.New("any arguments changed") }
	return nil
}

func VariadicError(ctx context.Context, values ...error) error {
	_, child := otel.Tracer("probe").Start(ctx, "error-child")
	child.End()
	if len(values) != 1 || values[0] == nil { return errors.New("error arguments changed") }
	return nil
}

func VariadicByte(ctx context.Context, values ...byte) error {
	_, child := otel.Tracer("probe").Start(ctx, "byte-child")
	child.End()
	if len(values) != 2 || values[0] != 1 || values[1] != 2 { return errors.New("byte arguments changed") }
	return nil
}
`)...)

	err = directory.WriteFile("ops/ops.go", data, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	mainSource, err := directory.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}

	replacement := `	if total, err := ops.Variadic(ctx, 1, 2, 3); err != nil || total != 6 {
		panic("variadic behavior changed")
	}
	if err := ops.VariadicString(ctx, "a", "b"); err != nil { panic(err) }
	if err := ops.VariadicAny(ctx, 1, "b"); err != nil { panic(err) }
	if err := ops.VariadicError(ctx, context.Canceled); err != nil { panic(err) }
	if err := ops.VariadicByte(ctx, 1, 2); err != nil { panic(err) }
	root.End()`

	modified := strings.Replace(string(mainSource), "\troot.End()", replacement, 1)
	if modified == string(mainSource) {
		t.Fatal("variadic fixture insertion point missing")
	}

	err = directory.WriteFile("main.go", []byte(modified), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	policy := []byte(`apiVersion: otelplan.io/v1alpha1
kind: InstrumentationPlan
backend: {name: otelc, version: v1.1.0}
project: {packages: [./ops]}
rules:
- id: variadic
  match: {symbols: [example.com/probe/ops.Variadic]}
  span: {name: variadic}
- id: variadic-string
  match: {symbols: [example.com/probe/ops.VariadicString]}
  span: {name: variadic-string}
- id: variadic-any
  match: {symbols: [example.com/probe/ops.VariadicAny]}
  span: {name: variadic-any}
- id: variadic-error
  match: {symbols: [example.com/probe/ops.VariadicError]}
  span: {name: variadic-error}
- id: variadic-byte
  match: {symbols: [example.com/probe/ops.VariadicByte]}
  span: {name: variadic-byte}
`)

	err = directory.WriteFile("otelplan.yaml", policy, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return map[string][]byte{"ops/ops.go": data, "main.go": []byte(modified), "otelplan.yaml": policy}
}

func checkVariadicSourceUnchanged(t *testing.T, root string, original map[string][]byte) {
	t.Helper()

	directory := openIntegrationDirectory(t, root)
	defer func() { _ = directory.Close() }()

	for name, want := range original {
		actual, err := directory.ReadFile(name)
		if err != nil || !bytes.Equal(actual, want) {
			t.Fatalf("variadic build changed source file %s: %v", name, err)
		}
	}
}

func checkVariadicTrace(t *testing.T, output []byte) {
	t.Helper()

	var trace struct {
		Spans []struct {
			Name   string `json:"Name"`
			ID     string `json:"ID"`
			Parent string `json:"Parent"`
			Trace  string `json:"Trace"`
		} `json:"Spans"`
	}

	err := json.Unmarshal(output, &trace)
	if err != nil {
		t.Fatalf("decode variadic trace: %v", err)
	}

	spans := make(map[string]struct{ ID, Parent, Trace string }, len(trace.Spans))
	for _, span := range trace.Spans {
		spans[span.Name] = struct{ ID, Parent, Trace string }{span.ID, span.Parent, span.Trace}
	}

	root := spans["root"]
	if root.ID == "" {
		t.Fatalf("root span missing: %s", output)
	}

	for _, target := range []struct{ operation, child string }{
		{operation: "variadic", child: "variadic-child"},
		{operation: "variadic-string", child: "string-child"},
		{operation: "variadic-any", child: "any-child"},
		{operation: "variadic-error", child: "error-child"},
		{operation: "variadic-byte", child: "byte-child"},
	} {
		operation, child := spans[target.operation], spans[target.child]
		if operation.ID == "" || child.ID == "" ||
			operation.Parent != root.ID || child.Parent != operation.ID ||
			operation.Trace != root.Trace || child.Trace != root.Trace {
			t.Fatalf("variadic spans have incorrect parent or trace: %s", output)
		}
	}
}
