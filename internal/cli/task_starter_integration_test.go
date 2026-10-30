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
	taskModule    = "github.com/go-task/task/v3"
	taskVersion   = "v3.44.1"
	taskModuleSum = "h1:i+lP9Ng3SQVUdBR8mmuiID7uKoIu+5LwwkOjio92VEQ="
	taskGoModSum  = "h1:mEX7XBXFUGlbIu8Aqv3r8N5LYYdHJ7PTm+Ak6bSuIvo="
	taskPolicy    = "otelplan.acceptance.yaml"
)

func TestTaskStarterAcceptance(t *testing.T) {
	if os.Getenv("OTELPLAN_OTELC") == "" {
		t.Skip("OTELPLAN_OTELC is required for pinned OSS acceptance")
	}

	cache := t.TempDir()
	t.Setenv("GOMODCACHE", cache)
	testenv.ConfigureGoCache(t, cache)
	root := prepareTaskSource(t, cache)
	testenv.DenyNetwork(t)
	original := testenv.SnapshotFiles(t, root)

	args := []string{"init", "--offline", "--non-interactive", "--dry-run", "--format=json", "--output=" + taskPolicy}
	first := runTaskCLI(t, root, args...)

	second := runTaskCLI(t, root, args...)
	if !bytes.Equal(first, second) {
		t.Fatal("repeated Task init previews differ")
	}

	preview := parseTaskPreview(t, first)
	checkTaskSuggestions(t, preview.Suggestions)
	testenv.AssertFilesUnchanged(t, original, testenv.SnapshotFiles(t, root))

	runTaskCLI(t, root, "init", "--offline", "--non-interactive", "--format=json", "--output="+taskPolicy)
	directory := openLockTestRoot(t, root)

	written, err := directory.ReadFile(taskPolicy)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(written, []byte(preview.Policy)) {
		t.Fatal("written Task policy differs from preview")
	}

	ruleIDs := checkTaskPolicy(t, root, preview.Suggestions)
	withPolicy := testenv.SnapshotFiles(t, root)
	assertTaskFilesUnchanged(t, original, withPolicy, taskPolicy)

	config := "--config=" + taskPolicy
	runTaskCLI(t, root, "validate", config, "--offline", "--strict", "--format=json")
	inspect := runTaskCLI(t, root, "inspect", config, "--offline", "--format=json")
	checkTaskInspection(t, inspect, preview.Suggestions, ruleIDs)
	testenv.AssertFilesUnchanged(t, withPolicy, testenv.SnapshotFiles(t, root))

	runTaskCLI(t, root, "lock", config, "--offline", "--format=json")

	firstLock, err := directory.ReadFile("otelplan.lock")
	if err != nil {
		t.Fatal(err)
	}

	runTaskCLI(t, root, "lock", config, "--offline", "--format=json")

	secondLock, err := directory.ReadFile("otelplan.lock")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(firstLock, secondLock) {
		t.Fatal("repeated Task lock writes differ")
	}

	withLock := testenv.SnapshotFiles(t, root)
	assertTaskFilesUnchanged(t, original, withLock, taskPolicy, "otelplan.lock")
	runTaskCLI(t, root, "lock", config, "--offline", "--check", "--format=json")
	runTaskCLI(t, root, "diff", config, "--offline", "--check", "--format=json")
	testenv.AssertFilesUnchanged(t, withLock, testenv.SnapshotFiles(t, root))
}

type taskPreview struct {
	Policy      string
	Suggestions []suggest.Suggestion
}

func parseTaskPreview(t *testing.T, output []byte) taskPreview {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil || !reply.OK {
		t.Fatalf("invalid Task init response: %v %s", err, output)
	}

	var data struct {
		Policy      string               `json:"policy"`
		DryRun      bool                 `json:"dryRun"`
		Suggestions []suggest.Suggestion `json:"suggestions"`
	}

	err = json.Unmarshal(reply.Data, &data)
	if err != nil || !data.DryRun || data.Policy == "" {
		t.Fatalf("invalid Task preview data: %v %s", err, output)
	}

	return taskPreview{Policy: data.Policy, Suggestions: data.Suggestions}
}

func checkTaskSuggestions(t *testing.T, suggestions []suggest.Suggestion) {
	t.Helper()

	if len(suggestions) == 0 || len(suggestions) > 20 {
		t.Fatalf("Task starter selected %d suggestions, want 1..20", len(suggestions))
	}

	expected := map[string]bool{
		"github.com/go-task/task/v3.(*Executor).RunTask":              false,
		"github.com/go-task/task/v3/internal/execext.RunCommand":      false,
		"github.com/go-task/task/v3/taskfile.(*Reader).Read":          false,
		"github.com/go-task/task/v3/taskfile.(*HTTPNode).ReadContext": false,
	}

	seen := map[model.SymbolID]bool{}
	for _, suggestion := range suggestions {
		checkTaskSuggestion(t, suggestion, expected, seen)
	}

	for symbol, found := range expected {
		if !found {
			t.Fatalf("Task starter omitted expected operation %s", symbol)
		}
	}
}

func checkTaskSuggestion(
	t *testing.T, suggestion suggest.Suggestion, expected map[string]bool, seen map[model.SymbolID]bool,
) {
	t.Helper()

	symbol := string(suggestion.SymbolID)
	if seen[suggestion.SymbolID] || !taskSymbolOwned(symbol) {
		t.Fatalf("invalid or duplicate Task suggestion: %+v", suggestion)
	}

	seen[suggestion.SymbolID] = true

	if _, found := expected[symbol]; found {
		expected[symbol] = true
	}

	checkTaskEvidence(t, suggestion, symbol)
}

func checkTaskEvidence(t *testing.T, suggestion suggest.Suggestion, symbol string) {
	t.Helper()

	if symbol == taskModule+".(*Executor).Run" || symbol == taskModule+".(*Executor).Status" {
		t.Fatalf("variadic Executor method selected: %s", suggestion.SymbolID)
	}

	evidence := strings.Join(suggestion.Evidence, " ")

	if symbol == "github.com/go-task/task/v3.(*Executor).RunTask" &&
		(!strings.Contains(evidence, "reachable from main.main") || !strings.Contains(evidence, "static call(s)")) {
		t.Fatalf("RunTask lacks static reachability evidence: %+v", suggestion)
	}

	if symbol == "github.com/go-task/task/v3/taskfile.(*HTTPNode).ReadContext" &&
		(!strings.Contains(evidence, "interface method") || !strings.Contains(evidence, "infrastructure")) {
		t.Fatalf("HTTPNode.ReadContext lacks interface/infrastructure evidence: %+v", suggestion)
	}
}

func taskSymbolOwned(symbol string) bool {
	return strings.HasPrefix(symbol, taskModule+".") || strings.HasPrefix(symbol, taskModule+"/")
}

func checkTaskPolicy(t *testing.T, root string, suggestions []suggest.Suggestion) map[model.SymbolID]string {
	t.Helper()

	parsed, err := policy.Load(filepath.Join(root, taskPolicy))
	if err != nil || policy.Validate(parsed).HasErrors() {
		t.Fatalf("invalid Task starter policy: %v", err)
	}

	checkTaskPolicyDefaults(t, parsed, len(suggestions))

	bySymbol := map[model.SymbolID]suggest.Suggestion{}
	for _, suggestion := range suggestions {
		bySymbol[suggestion.SymbolID] = suggestion
	}

	ruleIDs := map[model.SymbolID]string{}

	for _, rule := range parsed.Rules {
		checkTaskRule(t, rule, bySymbol)

		symbol := model.SymbolID(rule.Match.Symbols[0])

		if _, exists := ruleIDs[symbol]; exists {
			t.Fatalf("duplicate Task policy symbol: %s", symbol)
		}

		ruleIDs[symbol] = rule.ID
	}

	return ruleIDs
}

func checkTaskPolicyDefaults(t *testing.T, parsed *model.Policy, count int) {
	t.Helper()

	if parsed.Backend.Name != model.BackendNameOTelC || parsed.Backend.Version != "v1.1.0" ||
		parsed.Defaults.Context.Mode != model.ContextModeRequire || !parsed.Defaults.Errors.Record ||
		parsed.Defaults.Attributes.Arguments || parsed.Defaults.Attributes.Results || len(parsed.Rules) != count {
		t.Fatalf("unsafe or incomplete Task starter policy: %+v", parsed)
	}
}

func checkTaskRule(t *testing.T, rule model.Rule, bySymbol map[model.SymbolID]suggest.Suggestion) {
	t.Helper()

	if len(rule.Match.Symbols) != 1 || rule.Description == "" || !strings.HasPrefix(rule.ID, "suggest-") ||
		len(rule.Attributes) != 0 {
		t.Fatalf("Task starter rule is not exact and safe: %+v", rule)
	}

	suggestion, ok := bySymbol[model.SymbolID(rule.Match.Symbols[0])]
	if !ok || !strings.Contains(rule.Description, suggestion.Confidence) || !strings.Contains(rule.Description, "score") {
		t.Fatalf("Task rule provenance does not match suggestion: %+v", rule)
	}

	for _, evidence := range suggestion.Evidence {
		if !strings.Contains(rule.Description, evidence) {
			t.Fatalf("Task rule description lost suggestion evidence %q: %+v", evidence, rule)
		}
	}
}

func checkTaskInspection(
	t *testing.T, output []byte, suggestions []suggest.Suggestion, ruleIDs map[model.SymbolID]string,
) {
	t.Helper()

	plan := decodeTaskInspection(t, output)

	if len(plan.Targets) != len(suggestions) {
		t.Fatalf("Task inspect selected %d targets, want %d", len(plan.Targets), len(suggestions))
	}

	selected := map[model.SymbolID]bool{}

	for index := range plan.Targets {
		target := &plan.Targets[index]
		checkTaskTarget(t, *target, selected, ruleIDs)
	}

	for _, suggestion := range suggestions {
		if !selected[suggestion.SymbolID] {
			t.Fatalf("Task inspect omitted %s", suggestion.SymbolID)
		}
	}
}

func decodeTaskInspection(t *testing.T, output []byte) model.ResolvedPlan {
	t.Helper()

	var reply globalArgumentReply

	err := json.Unmarshal(output, &reply)
	if err != nil || !reply.OK {
		t.Fatalf("invalid Task inspect response: %v %s", err, output)
	}

	var plan model.ResolvedPlan

	err = json.Unmarshal(reply.Data, &plan)
	if err != nil {
		t.Fatalf("decode Task inspect plan: %v", err)
	}

	return plan
}

func checkTaskTarget(
	t *testing.T, target model.ResolvedTarget, selected map[model.SymbolID]bool, ruleIDs map[model.SymbolID]string,
) {
	t.Helper()

	if selected[target.SymbolID] || target.RuleID == "" || target.RuleID != ruleIDs[target.SymbolID] ||
		len(target.Attributes) != 0 ||
		target.ContextStrategy.Strategy != model.ContextStrategyArgument || !target.ErrorStrategy.Record {
		t.Fatalf("invalid Task resolved target: %+v", target)
	}

	selected[target.SymbolID] = true
}

func assertTaskFilesUnchanged(t *testing.T, original, current map[string][]byte, permitted ...string) {
	t.Helper()

	expected := maps.Clone(original)

	for _, name := range permitted {
		contents, found := current[name]
		if !found {
			t.Fatalf("Task CLI did not create permitted output %s", name)
		}

		expected[name] = contents
	}

	testenv.AssertFilesUnchanged(t, expected, current)
}

func runTaskCLI(t *testing.T, root string, command ...string) []byte {
	t.Helper()

	args := append([]string{"--root", root}, command...)

	var stdout, stderr bytes.Buffer
	if exit := cli.Run(t.Context(), args, &stdout, &stderr); exit != 0 || stderr.Len() != 0 {
		t.Fatalf("Task CLI %v exit=%d stdout=%s stderr=%s", command, exit, &stdout, &stderr)
	}

	return bytes.Clone(stdout.Bytes())
}

func prepareTaskSource(t *testing.T, cache string) string {
	t.Helper()
	seed := t.TempDir()
	directory := openLockTestRoot(t, seed)
	writeLockTestFile(t, directory, "go.mod", "module example.com/task-seed\n\ngo 1.27\n")
	command := exec.CommandContext(t.Context(), "go", "mod", "download", "-json", taskModule+"@"+taskVersion)
	command.Dir = seed

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("download Task module: %v: %s", err, output)
	}

	var download struct {
		Path     string `json:"Path"`
		Version  string `json:"Version"`
		Sum      string `json:"Sum"`
		GoModSum string `json:"GoModSum"`
	}

	err = json.Unmarshal(output, &download)
	if err != nil {
		t.Fatalf("decode Task module identity: %v", err)
	}

	if download.Path != taskModule || download.Version != taskVersion || download.Sum != taskModuleSum ||
		download.GoModSum != taskGoModSum {
		t.Fatalf("unexpected Task module identity: %+v", download)
	}

	source := filepath.Join(cache, "github.com", "go-task", "task", "v3@"+taskVersion)

	root := t.TempDir()

	err = os.CopyFS(root, os.DirFS(source))
	if err != nil {
		t.Fatalf("copy verified Task source: %v", err)
	}

	before := testenv.SnapshotFiles(t, root)
	dependencySeed := t.TempDir()
	seedRoot := openLockTestRoot(t, dependencySeed)

	copyTaskManifests(t, root, seedRoot)

	prepare := exec.CommandContext(t.Context(), "go", "mod", "download", "all")

	prepare.Dir = dependencySeed

	output, err = prepare.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare copied Task dependencies: %v: %s", err, output)
	}

	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, root))

	return root
}

func copyTaskManifests(t *testing.T, root string, destination *os.Root) {
	t.Helper()

	source := openLockTestRoot(t, root)

	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := source.ReadFile(name)
		if err != nil {
			t.Fatalf("read Task manifest %s: %v", name, err)
		}

		writeLockTestFile(t, destination, name, string(data))
	}
}
