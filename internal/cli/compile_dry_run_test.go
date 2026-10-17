package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

type compileDryRunResponse struct {
	OK   bool `json:"ok"`
	Data struct {
		Path      string               `json:"path"`
		Files     int                  `json:"files"`
		Backend   model.LockBackend    `json:"backend"`
		DryRun    bool                 `json:"dryRun"`
		Artifacts []model.ArtifactFile `json:"artifacts"`
	} `json:"data"`
}

func TestCompileDryRunPreviewsArtifactsWithoutPublishing(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required for pinned backend integration")
	}

	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root, analysisCache, cacheBefore, original := prepareCompileDryRunFixture(t)

	args := []string{"compile", "--root", root, "--dry-run", "--offline", "--format=json",
		"--cache-dir", analysisCache}
	firstOut := runCLI(t, args)
	first := decodeCompileDryRun(t, firstOut)
	assertDryRunResponse(t, root, first)
	assertCacheAndProjectUnchanged(t, root, analysisCache, cacheBefore, original)

	secondOut := runCLI(t, args)
	if string(secondOut) != string(firstOut) {
		t.Fatalf("repeated dry-run changed response:\nfirst=%ssecond=%s", firstOut, secondOut)
	}

	verifyCustomDryRun(t, root)
	verifyNormalCompileParity(t, root, first.Data.Artifacts)
	verifyTextDryRun(t, root, first.Data.Artifacts)
	assertCacheAndProjectUnchanged(t, root, analysisCache, cacheBefore, original)
}

func prepareCompileDryRunFixture(t *testing.T) (string, string, map[string][]byte, map[string][]byte) {
	t.Helper()

	testenv.PrepareOffline(t, "../..", filepath.Join("testdata", "architectures", "common"))
	root, _ := lockContractFixture(t)
	analysisCache := filepath.Join(root, "analysis-cache")

	var scanOut, scanErr bytes.Buffer

	scanArgs := []string{"scan", "--root", root, "--offline", "--format=json", "--cache-dir", analysisCache}
	if code := cli.Run(t.Context(), scanArgs, &scanOut, &scanErr); code != 0 {
		t.Fatalf("warm-cache scan exit=%d: %s %s", code, &scanOut, &scanErr)
	}

	cacheBefore := testenv.SnapshotFiles(t, analysisCache)
	if len(cacheBefore) == 0 {
		t.Fatal("warm analysis cache is empty")
	}

	return root, analysisCache, cacheBefore, testenv.SnapshotFiles(t, root)
}

func runCLI(t *testing.T, args []string) []byte {
	t.Helper()

	var stdout, stderr bytes.Buffer
	if code := cli.Run(t.Context(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("CLI exit=%d: %s %s", code, &stdout, &stderr)
	}

	return stdout.Bytes()
}

func decodeCompileDryRun(t *testing.T, output []byte) compileDryRunResponse {
	t.Helper()

	var reply compileDryRunResponse

	err := json.Unmarshal(output, &reply)
	if err != nil {
		t.Fatalf("decode dry-run response: %v", err)
	}

	return reply
}

func assertDryRunResponse(t *testing.T, root string, reply compileDryRunResponse) {
	t.Helper()

	if !reply.OK || !reply.Data.DryRun || reply.Data.Files == 0 || reply.Data.Backend.Digest == "" ||
		len(reply.Data.Artifacts) != reply.Data.Files {
		t.Fatalf("invalid dry-run response: %+v", reply)
	}

	if !filepath.IsAbs(reply.Data.Path) {
		t.Fatalf("dry-run path is not absolute: %q", reply.Data.Path)
	}

	_, err := os.Stat(reply.Data.Path)
	if !os.IsNotExist(err) {
		t.Fatalf("dry-run published output: stat=%v", err)
	}

	_, err = os.Stat(filepath.Join(root, ".otelplan"))
	if !os.IsNotExist(err) {
		t.Fatalf("dry-run created default output directory: stat=%v", err)
	}
}

func assertCacheAndProjectUnchanged(t *testing.T, root, analysisCache string,
	cacheBefore, original map[string][]byte,
) {
	t.Helper()

	testenv.AssertFilesUnchanged(t, cacheBefore, testenv.SnapshotFiles(t, analysisCache))
	testenv.AssertFilesUnchanged(t, original, testenv.SnapshotFiles(t, root))
}

func verifyCustomDryRun(t *testing.T, root string) {
	t.Helper()

	customOutput := filepath.Join("nested", "generated")
	missingCache := filepath.Join("nested", "cache")
	args := []string{"compile", "--root", root, "--dry-run", "--offline", "--format=json",
		"--output", customOutput, "--cache-dir", missingCache}
	output := runCLI(t, args)

	var reply struct {
		Data struct {
			Path string `json:"path"`
		} `json:"data"`
	}

	err := json.Unmarshal(output, &reply)
	if err != nil {
		t.Fatalf("decode custom dry-run response: %v", err)
	}

	if want := filepath.Join(root, customOutput); reply.Data.Path != want {
		t.Fatalf("custom dry-run path=%q, want %q", reply.Data.Path, want)
	}

	_, err = os.Stat(filepath.Join(root, "nested"))
	if !os.IsNotExist(err) {
		t.Fatalf("custom dry-run created output/cache parent: stat=%v", err)
	}
}

func verifyNormalCompileParity(t *testing.T, root string, preview []model.ArtifactFile) {
	t.Helper()

	normalOutput := filepath.Join(t.TempDir(), "normal-generated")
	args := []string{"compile", "--root", root, "--offline", "--format=json", "--output", normalOutput}
	output := runCLI(t, args)

	var reply struct {
		Data map[string]json.RawMessage `json:"data"`
	}

	err := json.Unmarshal(output, &reply)
	if err != nil {
		t.Fatalf("decode normal compile response: %v", err)
	}

	if _, ok := reply.Data["dryRun"]; ok {
		t.Fatal("normal compile JSON unexpectedly contains dryRun")
	}

	if _, ok := reply.Data["artifacts"]; ok {
		t.Fatal("normal compile JSON unexpectedly contains artifacts")
	}

	published, err := compiler.ReadArtifacts(normalOutput)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(published.Files, preview) {
		t.Fatalf("preview inventory differs from published inventory: preview=%+v published=%+v",
			preview, published.Files)
	}
}

func verifyTextDryRun(t *testing.T, root string, artifacts []model.ArtifactFile) {
	t.Helper()

	args := []string{"compile", "--root", root, "--dry-run", "--offline", "--format=text"}
	output := runCLI(t, args)

	text := string(output)
	if !strings.Contains(text, "would write") {
		t.Fatalf("text dry-run omitted write preview: %s", text)
	}

	for _, artifact := range artifacts {
		if !strings.Contains(text, artifact.Path) || !strings.Contains(text, artifact.Digest) {
			t.Fatalf("text dry-run omitted artifact %s: %s", artifact.Path, text)
		}
	}
}
