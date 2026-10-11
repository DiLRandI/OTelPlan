package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestArchitectureTracesWithPinnedBackend(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required")
	}

	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))

	for _, mode := range []struct {
		name    string
		offline bool
	}{
		{name: "cold-cache-online", offline: false},
		{name: "prepared-cache-offline", offline: true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			cache := t.TempDir()
			t.Setenv("GOMODCACHE", cache)
			testenv.ConfigureGoCache(t, cache)

			if mode.offline {
				prepareOfflineArchitectureCache(t)
			}

			runArchitectureTraces(t, mode.offline)
		})
	}
}

func runArchitectureTraces(t *testing.T, offline bool) {
	t.Helper()

	for _, scenario := range []struct{ name, checkout, payment string }{
		{"package", "internal/service.(*Order).Submit", "internal/service.(User).Authorize"},
		{"feature", "internal/order.Submit", "internal/payment.Authorize"},
		{"hexagonal", "internal/application.(Checkout).Submit", "internal/adapters.(*Gateway).Authorize"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := architectureFixture(t, scenario.name)
			original := testenv.SnapshotFiles(t, root)
			inspected := invokeArchitecture(t, root, offline, 0, "inspect")
			checkArchitectureTargets(t, inspected, scenario.checkout, scenario.payment)

			invokeArchitecture(t, root, offline, 0, "lock")
			original["otelplan.lock"] = testenv.SnapshotFiles(t, root)["otelplan.lock"]

			invokeArchitecture(t, root, offline, 0, "validate", "--strict")
			invokeArchitecture(t, root, offline, 0, "compile", "--output", filepath.Join(t.TempDir(), "generated"))
			binary := filepath.Join(t.TempDir(), "app")
			invokeArchitecture(t, root, offline, 0, "build", "--", "-race", "-buildvcs=false", "-o", binary, ".")

			output, err := exec.CommandContext(t.Context(), binary).Output()
			if err != nil {
				t.Fatalf("run instrumented architecture fixture: %v", err)
			}

			checkArchitectureTrace(t, output)
			invokeArchitecture(t, root, offline, 0, "lock", "--check")
			testenv.AssertFilesUnchanged(t, original, testenv.SnapshotFiles(t, root))
		})
	}
}

func checkArchitectureTargets(t *testing.T, inspected globalArgumentReply, checkout, payment string) {
	t.Helper()

	data, err := json.Marshal(inspected.Data)
	if err != nil {
		t.Fatalf("encode inspected architecture plan: %v", err)
	}

	var plan model.ResolvedPlan

	err = json.Unmarshal(data, &plan)
	if err != nil {
		t.Fatalf("decode inspected architecture plan: %v", err)
	}

	want := map[string]string{
		"checkout": "example.com/architecture/" + checkout,
		"payment":  "example.com/architecture/" + payment,
	}
	if len(plan.Targets) != len(want) {
		t.Fatalf("unexpected architecture targets: %s", data)
	}

	for _, target := range plan.Targets {
		if want[target.SpanName] != string(target.SymbolID) || target.RuleID != target.SpanName {
			t.Fatalf("unexpected architecture target: %+v", target)
		}

		delete(want, target.SpanName)
	}
}

type architectureSpan struct {
	Name       string         `json:"Name"`
	ID         string         `json:"ID"`
	Parent     string         `json:"Parent"`
	Trace      string         `json:"Trace"`
	Scope      string         `json:"Scope"`
	Kind       string         `json:"Kind"`
	Error      bool           `json:"Error"`
	Events     int            `json:"Events"`
	Attributes map[string]any `json:"Attributes"`
}

type architectureTrace struct {
	ReturnedError string             `json:"ReturnedError"`
	Spans         []architectureSpan `json:"Spans"`
}

func checkArchitectureTrace(t *testing.T, output []byte) {
	t.Helper()

	var result architectureTrace

	err := json.Unmarshal(output, &result)
	if err != nil {
		t.Fatalf("decode architecture trace: %v", err)
	}

	if result.ReturnedError != "declined" || len(result.Spans) != 4 {
		t.Fatalf("application behavior or exact instrumentation changed: %s", output)
	}

	if strings.Contains(string(output), "private-token-never-capture") {
		t.Fatal("private argument captured")
	}

	byName := indexArchitectureSpans(t, result.Spans, output)
	checkArchitectureParentage(t, byName, output)
}

func indexArchitectureSpans(t *testing.T, spans []architectureSpan, output []byte) map[string]architectureSpan {
	t.Helper()

	byName := map[string]architectureSpan{}

	ids := map[string]bool{}

	for _, item := range spans {
		if _, exists := byName[item.Name]; exists || item.ID == "0000000000000000" || ids[item.ID] {
			t.Fatalf("duplicate or invalid span: %s", output)
		}

		byName[item.Name], ids[item.ID] = item, true

		checkArchitectureCapture(t, item, output)
	}

	return byName
}

func checkArchitectureCapture(t *testing.T, item architectureSpan, output []byte) {
	t.Helper()

	if (item.Name == "root" || item.Name == "downstream") && (item.Error || item.Events != 0) {
		t.Fatalf("manual span changed: %s", output)
	}

	if len(item.Attributes) != 0 {
		t.Fatalf("unexpected attribute capture: %s", output)
	}
}

func checkArchitectureParentage(t *testing.T, byName map[string]architectureSpan, output []byte) {
	t.Helper()

	parent, exists := byName["root"]
	if !exists || parent.Parent != "0000000000000000" || parent.Trace == "00000000000000000000000000000000" {
		t.Fatalf("invalid root span: %s", output)
	}

	for _, name := range []string{"checkout", "payment", "downstream"} {
		child, exists := byName[name]
		if !exists || child.Parent != parent.ID || child.Trace != parent.Trace {
			t.Fatalf("broken trace parentage for %s: %s", name, output)
		}

		if name != "downstream" {
			checkArchitectureBusinessSpan(t, child, output)
		}

		parent = child
	}
}

func checkArchitectureBusinessSpan(t *testing.T, item architectureSpan, output []byte) {
	t.Helper()

	if !item.Error || item.Events != 1 || item.Scope != "otelplan.io/business" || item.Kind != "internal" {
		t.Fatalf("business span semantics changed: %s", output)
	}
}

func architectureFixture(t *testing.T, architecture string) string {
	t.Helper()

	root := t.TempDir()
	for _, part := range []string{"common", architecture} {
		err := os.CopyFS(root, os.DirFS(filepath.Join("testdata", "architectures", part)))
		if err != nil {
			t.Fatalf("copy %s architecture fixture: %v", part, err)
		}
	}

	return root
}

func prepareOfflineArchitectureCache(t *testing.T) {
	t.Helper()

	const discoveryFailureExit = 4

	seed := architectureFixture(t, "package")

	rejected := invokeArchitecture(t, seed, true, discoveryFailureExit, "inspect")
	if len(rejected.Diagnostics) != 1 || rejected.Diagnostics[0].Code != model.CodeUnresolvedSymbol ||
		!strings.Contains(rejected.Diagnostics[0].Message, "module lookup disabled by GOPROXY=off") {
		t.Fatalf("cold offline discovery failed for an unexpected reason: %+v", rejected.Diagnostics)
	}

	command := exec.CommandContext(t.Context(), "go", "mod", "download", "all")
	command.Dir = seed

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare pinned architecture dependencies: %v: %s", err, output)
	}

	testenv.DenyNetwork(t)
}

func invokeArchitecture(t *testing.T, root string, offline bool, want int, args ...string) globalArgumentReply {
	t.Helper()

	arguments := []string{"--root", root, "--format=json"}
	if offline {
		arguments = append(arguments, "--offline")
	}

	arguments = append(arguments, args...)

	var output, stderr bytes.Buffer

	code := cli.Run(t.Context(), arguments, &output, &stderr)
	if code != want {
		t.Fatalf("%v exit=%d want=%d output=%s stderr=%s", args, code, want, &output, &stderr)
	}

	var reply globalArgumentReply

	err := json.Unmarshal(output.Bytes(), &reply)
	if err != nil {
		t.Fatalf("invalid architecture command JSON: %v: %s", err, &output)
	}

	if reply.OK != (want == 0) {
		t.Fatalf("architecture command JSON disagrees with exit: %+v", reply)
	}

	return reply
}
