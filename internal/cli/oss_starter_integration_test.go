package cli_test

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/policy"
	"github.com/DiLRandI/OTelPlan/internal/suggest"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const (
	memosModule    = "github.com/usememos/memos"
	memosVersion   = "v0.26.0"
	memosModuleSum = "h1:cv1T2imOksNwh5OoBrjhQ6vou8Obk4k40qxbQ1/mlak="
	memosGoModSum  = "h1:DFsWGeYG3cS9znYf154wgE0CyE7XengDf/934LifDx4="
	memosPolicy    = "otelplan.acceptance.yaml"
)

func TestMemosStarterAcceptance(t *testing.T) {
	if os.Getenv("OTELPLAN_OTELC") == "" {
		t.Skip("OTELPLAN_OTELC is required for pinned OSS acceptance")
	}

	cache := t.TempDir()
	t.Setenv("GOMODCACHE", cache)
	testenv.ConfigureGoCache(t, cache)
	root := prepareMemosSource(t, cache)
	testenv.DenyNetwork(t)
	original := testenv.SnapshotFiles(t, root)

	previewArgs := []string{"init", "--offline", "--non-interactive", "--dry-run",
		"--format=json", "--output=" + memosPolicy}
	first := runMemosCLI(t, root, previewArgs...)

	second := runMemosCLI(t, root, previewArgs...)
	if !bytes.Equal(first, second) {
		t.Fatal("repeated Memos init previews differ")
	}

	preview := parseMemosPreview(t, first)
	checkMemosSuggestions(t, preview.Suggestions)
	testenv.AssertFilesUnchanged(t, original, testenv.SnapshotFiles(t, root))

	runMemosCLI(t, root, "init", "--offline", "--non-interactive", "--format=json", "--output="+memosPolicy)
	directory := openLockTestRoot(t, root)

	generated, err := directory.ReadFile(memosPolicy)
	if err != nil {
		t.Fatal(err)
	}

	if string(generated) != preview.Policy {
		t.Fatal("written Memos policy differs from preview")
	}

	checkMemosPolicy(t, root, preview.Suggestions)
	withPolicy := testenv.SnapshotFiles(t, root)
	assertMemosFilesUnchanged(t, original, withPolicy, memosPolicy)

	config := "--config=" + memosPolicy
	runMemosCLI(t, root, "validate", config, "--offline", "--strict", "--format=json")
	inspect := runMemosCLI(t, root, "inspect", config, "--offline", "--format=json")
	checkMemosInspection(t, inspect, preview.Suggestions)
	testenv.AssertFilesUnchanged(t, withPolicy, testenv.SnapshotFiles(t, root))

	runMemosCLI(t, root, "lock", config, "--offline", "--format=json")

	firstLock, err := directory.ReadFile("otelplan.lock")
	if err != nil {
		t.Fatal(err)
	}

	runMemosCLI(t, root, "lock", config, "--offline", "--format=json")

	secondLock, err := directory.ReadFile("otelplan.lock")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(firstLock, secondLock) {
		t.Fatal("repeated Memos lock writes differ")
	}

	withLock := testenv.SnapshotFiles(t, root)
	assertMemosFilesUnchanged(t, original, withLock, memosPolicy, "otelplan.lock")
	runMemosCLI(t, root, "lock", config, "--offline", "--check", "--format=json")
	runMemosCLI(t, root, "diff", config, "--offline", "--check", "--format=json")
	testenv.AssertFilesUnchanged(t, withLock, testenv.SnapshotFiles(t, root))
}

type memosPreview struct {
	Policy      string
	DryRun      bool
	Suggestions []suggest.Suggestion
}

func parseMemosPreview(t *testing.T, output []byte) memosPreview {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil || !reply.OK {
		t.Fatalf("invalid Memos init response: %v %s", err, output)
	}

	var data struct {
		Policy      string               `json:"policy"`
		DryRun      bool                 `json:"dryRun"`
		Suggestions []suggest.Suggestion `json:"suggestions"`
	}

	err = json.Unmarshal(reply.Data, &data)
	if err != nil || !data.DryRun || data.Policy == "" {
		t.Fatalf("invalid Memos preview data: %v %s", err, output)
	}

	return memosPreview{Policy: data.Policy, DryRun: data.DryRun, Suggestions: data.Suggestions}
}

func checkMemosSuggestions(t *testing.T, suggestions []suggest.Suggestion) {
	t.Helper()

	if len(suggestions) == 0 || len(suggestions) > 20 {
		t.Fatalf("Memos starter selected %d suggestions, want 1..20", len(suggestions))
	}

	const prefix = "github.com/usememos/memos/server/router/api/v1.(*APIV1Service)."

	expected := map[string]bool{"CreateMemo": false, "DeleteMemo": false, "CreateUser": false, "GetAttachment": false}
	for _, suggestion := range suggestions {
		checkMemosSuggestion(t, suggestion, prefix, expected)
	}

	for name, found := range expected {
		if !found {
			t.Fatalf("Memos starter omitted expected API operation %s: %+v", name, suggestions)
		}
	}
}

func checkMemosSuggestion(t *testing.T, suggestion suggest.Suggestion, prefix string, expected map[string]bool) {
	t.Helper()

	symbolID := string(suggestion.SymbolID)
	evidence := strings.Join(suggestion.Evidence, " ")

	if !strings.HasPrefix(symbolID, prefix) || strings.Contains(symbolID, "/store/db/") {
		t.Fatalf("unexpected Memos starter target: %s", symbolID)
	}

	if !strings.Contains(evidence, "connectrpc.com/connect.NewUnaryHandler") ||
		!strings.Contains(evidence, "reachable from") || !strings.Contains(evidence, "static call(s)") {
		t.Fatalf("Memos target lacks Connect static provenance: %+v", suggestion)
	}

	for name := range expected {
		if strings.HasSuffix(symbolID, ")."+name) {
			expected[name] = true
		}
	}
}

func checkMemosPolicy(t *testing.T, root string, suggestions []suggest.Suggestion) {
	t.Helper()

	path := filepath.Join(root, memosPolicy)

	parsed, err := policy.Load(path)
	if err != nil || policy.Validate(parsed).HasErrors() {
		t.Fatalf("invalid Memos starter policy: %v", err)
	}

	checkMemosPolicyDefaults(t, parsed, len(suggestions))

	for _, rule := range parsed.Rules {
		checkMemosRule(t, rule)
	}
}

func checkMemosPolicyDefaults(t *testing.T, parsed *model.Policy, count int) {
	t.Helper()

	if parsed.Backend.Name != model.BackendNameOTelC || parsed.Backend.Version != "v1.1.0" ||
		parsed.Defaults.Context.Mode != model.ContextModeRequire || !parsed.Defaults.Errors.Record ||
		parsed.Defaults.Attributes.Arguments || parsed.Defaults.Attributes.Results || len(parsed.Rules) != count {
		t.Fatalf("unsafe or incomplete Memos starter policy: %+v", parsed)
	}
}

func checkMemosRule(t *testing.T, rule model.Rule) {
	t.Helper()

	if len(rule.Match.Symbols) != 1 || rule.Description == "" || !strings.HasPrefix(rule.ID, "suggest-") {
		t.Fatalf("Memos starter rule is not exact: %+v", rule)
	}
}

func checkMemosInspection(t *testing.T, output []byte, suggestions []suggest.Suggestion) {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil || !reply.OK {
		t.Fatalf("invalid Memos inspect response: %v %s", err, output)
	}

	var plan model.ResolvedPlan

	err = json.Unmarshal(reply.Data, &plan)
	if err != nil {
		t.Fatalf("decode Memos inspect plan: %v", err)
	}

	if len(plan.Targets) != len(suggestions) {
		t.Fatalf("Memos inspect selected %d targets, want %d", len(plan.Targets), len(suggestions))
	}

	selected := map[model.SymbolID]bool{}
	for _, target := range plan.Targets {
		checkMemosTarget(t, selected, target)
	}

	for _, suggestion := range suggestions {
		if !selected[suggestion.SymbolID] {
			t.Fatalf("Memos inspect omitted %s", suggestion.SymbolID)
		}
	}
}

func checkMemosTarget(t *testing.T, selected map[model.SymbolID]bool, target model.ResolvedTarget) {
	t.Helper()

	if selected[target.SymbolID] || target.RuleID == "" || len(target.Attributes) != 0 {
		t.Fatalf("invalid Memos resolved target: %+v", target)
	}

	selected[target.SymbolID] = true
}

func assertMemosFilesUnchanged(t *testing.T, original, current map[string][]byte, permitted ...string) {
	t.Helper()

	expected := maps.Clone(original)

	for _, name := range permitted {
		contents, found := current[name]
		if !found {
			t.Fatalf("Memos CLI did not create permitted output %s", name)
		}

		expected[name] = contents
	}

	testenv.AssertFilesUnchanged(t, expected, current)
}

func runMemosCLI(t *testing.T, root string, command ...string) []byte {
	t.Helper()

	args := append([]string{"--root", root}, command...)

	var stdout, stderr bytes.Buffer
	if exit := cli.Run(t.Context(), args, &stdout, &stderr); exit != 0 || stderr.Len() != 0 {
		t.Fatalf("Memos CLI %v exit=%d stdout=%s stderr=%s", command, exit, &stdout, &stderr)
	}

	return bytes.Clone(stdout.Bytes())
}

func prepareMemosSource(t *testing.T, cache string) string {
	t.Helper()
	downloadMemosModule(t)

	source := filepath.Join(cache, "github.com", "usememos", "memos@v0.26.0")
	root := t.TempDir()

	err := os.CopyFS(root, os.DirFS(source))
	if err != nil {
		t.Fatalf("copy verified Memos source: %v", err)
	}

	download := exec.CommandContext(t.Context(), "go", "mod", "download", "all")
	download.Dir = root

	output, err := download.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare copied Memos dependencies: %v: %s", err, output)
	}

	return root
}

func downloadMemosModule(t *testing.T) {
	t.Helper()

	directory := openLockTestRoot(t, t.TempDir())
	writeLockTestFile(t, directory, "go.mod", "module example.com/memos-seed\n\ngo 1.27\n")
	command := exec.CommandContext(t.Context(), "go", "mod", "download", "-json", "github.com/usememos/memos@v0.26.0")
	command.Dir = directory.Name()

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("download Memos module: %v: %s", err, output)
	}

	var download struct {
		Path     string `json:"Path"`
		Version  string `json:"Version"`
		Sum      string `json:"Sum"`
		GoModSum string `json:"GoModSum"`
	}

	err = json.Unmarshal(output, &download)
	if err != nil {
		t.Fatalf("decode Memos module identity: %v", err)
	}

	if download.Path != memosModule || download.Version != memosVersion ||
		download.Sum != memosModuleSum || download.GoModSum != memosGoModSum {
		t.Fatalf("unexpected Memos module identity: %+v", download)
	}
}
