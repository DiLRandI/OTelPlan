package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
			configureArchitectureCache(t, cache)

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
			original := snapshotArchitecture(t, root)
			inspected := invokeArchitecture(t, root, offline, 0, "inspect")
			checkArchitectureTargets(t, inspected, scenario.checkout, scenario.payment)

			invokeArchitecture(t, root, offline, 0, "lock")
			original["otelplan.lock"] = snapshotArchitecture(t, root)["otelplan.lock"]

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
			checkArchitectureImmutability(t, original, snapshotArchitecture(t, root))
		})
	}
}

func checkArchitectureTargets(t *testing.T, inspected response, checkout, payment string) {
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

func snapshotArchitecture(t *testing.T, root string) map[string][]byte {
	t.Helper()

	directory, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("open architecture fixture: %v", err)
	}

	t.Cleanup(func() {
		err := directory.Close()
		if err != nil {
			t.Errorf("close architecture fixture: %v", err)
		}
	})

	files := map[string][]byte{}

	err = fs.WalkDir(directory.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk architecture fixture: %w", walkErr)
		}

		if entry.IsDir() {
			return nil
		}

		data, readErr := directory.ReadFile(name)
		if readErr != nil {
			return fmt.Errorf("read architecture fixture: %w", readErr)
		}

		files[name] = data

		return nil
	})
	if err != nil {
		t.Fatalf("snapshot architecture fixture: %v", err)
	}

	return files
}

func checkArchitectureImmutability(t *testing.T, original, current map[string][]byte) {
	t.Helper()

	if len(original) != len(current) {
		t.Errorf("architecture fixture file count changed: got %d, want %d", len(current), len(original))
	}

	for name, want := range original {
		got, exists := current[name]
		if !exists || !bytes.Equal(got, want) {
			t.Errorf("changed or removed project file %s", name)
		}
	}

	for name := range current {
		if _, exists := original[name]; !exists {
			t.Errorf("unexpected project file %s", name)
		}
	}
}

func checkArchitectureTrace(t *testing.T, output []byte) {
	t.Helper()

	type span struct {
		Name, ID, Parent, Trace, Scope, Kind string
		Error                                bool
		Events                               int
		Attributes                           map[string]any
	}

	var result struct {
		ReturnedError string
		Spans         []span
	}

	err := json.Unmarshal(output, &result)
	if err != nil {
		t.Fatal(err)
	}

	if result.ReturnedError != "declined" || len(result.Spans) != 4 {
		t.Fatalf("application behavior or exact instrumentation changed: %s", output)
	}

	if strings.Contains(string(output), "private-token-never-capture") {
		t.Fatal("private argument captured")
	}

	byName := map[string]span{}

	ids := map[string]bool{}

	for _, item := range result.Spans {
		if _, exists := byName[item.Name]; exists || item.ID == "0000000000000000" || ids[item.ID] {
			t.Fatalf("duplicate or invalid span: %s", output)
		}

		byName[item.Name], ids[item.ID] = item, true

		if (item.Name == "root" || item.Name == "downstream") && (item.Error || item.Events != 0) {
			t.Fatalf("manual span changed: %s", output)
		}

		if len(item.Attributes) != 0 {
			t.Fatalf("unexpected attribute capture: %s", output)
		}
	}

	parent, exists := byName["root"]
	if !exists || parent.Parent != "0000000000000000" || parent.Trace == "00000000000000000000000000000000" {
		t.Fatalf("invalid root span: %s", output)
	}

	for _, name := range []string{"checkout", "payment", "downstream"} {
		child, exists := byName[name]
		if !exists || child.Parent != parent.ID || child.Trace != parent.Trace {
			t.Fatalf("broken trace parentage for %s: %s", name, output)
		}

		if name != "downstream" && (!child.Error || child.Events != 1 || child.Scope != "otelplan.io/business" || child.Kind != "internal") {
			t.Fatalf("business span semantics changed: %s", output)
		}

		parent = child
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

func configureArchitectureCache(t *testing.T, cache string) {
	t.Helper()

	t.Setenv("GOENV", "off")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOPACKAGESDRIVER", "off")
	t.Setenv("GOPROXY", "https://proxy.golang.org")
	t.Setenv("GOSUMDB", "sum.golang.org")
	t.Setenv("GOPRIVATE", "")
	t.Setenv("GONOPROXY", "none")
	t.Setenv("GONOSUMDB", "none")
	t.Setenv("GOVCS", "*:off")
	t.Setenv("GOTMPDIR", t.TempDir())

	t.Cleanup(func() {
		// Test contexts are canceled before cleanup; removing read-only module caches still needs a live context.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 30*time.Second)
		defer cancel()

		command := exec.CommandContext(ctx, "go", "clean", "-modcache")

		command.Env = append(os.Environ(), "GOMODCACHE="+cache)

		output, err := command.CombinedOutput()
		if err != nil {
			t.Errorf("clean architecture module cache: %v: %s", err, output)
		}
	})
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

	denyArchitectureNetwork(t)
}

func denyArchitectureNetwork(t *testing.T) {
	t.Helper()

	var requests atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)

		writer.WriteHeader(http.StatusServiceUnavailable)
	}))

	t.Cleanup(func() {
		server.Close()

		if count := requests.Load(); count != 0 {
			t.Errorf("offline architecture workflow made %d network requests", count)
		}
	})

	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("HTTP_PROXY", server.URL)
	t.Setenv("HTTPS_PROXY", server.URL)
	t.Setenv("ALL_PROXY", server.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("http_proxy", server.URL)
	t.Setenv("https_proxy", server.URL)
	t.Setenv("all_proxy", server.URL)
	t.Setenv("no_proxy", "")
}

func invokeArchitecture(t *testing.T, root string, offline bool, want int, args ...string) response {
	t.Helper()

	arguments := []string{"--root", root, "--format=json"}
	if offline {
		arguments = append(arguments, "--offline")
	}

	arguments = append(arguments, args...)

	var output, stderr bytes.Buffer

	code := Run(arguments, &output, &stderr)
	if code != want {
		t.Fatalf("%v exit=%d want=%d output=%s stderr=%s", args, code, want, &output, &stderr)
	}

	var reply response

	err := json.Unmarshal(output.Bytes(), &reply)
	if err != nil {
		t.Fatalf("invalid architecture command JSON: %v: %s", err, &output)
	}

	if reply.OK != (want == 0) {
		t.Fatalf("architecture command JSON disagrees with exit: %+v", reply)
	}

	return reply
}
