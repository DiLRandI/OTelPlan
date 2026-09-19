package cli

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestArchitectureTracesWithPinnedBackend(t *testing.T) {
	executable := os.Getenv("OTELPLAN_OTELC")
	if executable == "" {
		t.Skip("OTELPLAN_OTELC is required")
	}
	t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, tc := range []struct{ name, checkout, payment string }{
		{"package", "internal/service.(*Order).Submit", "internal/service.(User).Authorize"},
		{"feature", "internal/order.Submit", "internal/payment.Authorize"},
		{"hexagonal", "internal/application.(Checkout).Submit", "internal/adapters.(*Gateway).Authorize"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, part := range []string{"common", tc.name} {
				if err := os.CopyFS(root, os.DirFS(filepath.Join("testdata", "architectures", part))); err != nil {
					t.Fatal(err)
				}
			}
			original := map[string][]byte{}
			if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				data, err := os.ReadFile(path)
				original[path] = data
				return err
			}); err != nil {
				t.Fatal(err)
			}
			inspected := invoke(t, root, 0, "inspect")
			data, err := json.Marshal(inspected.Data)
			if err != nil {
				t.Fatal(err)
			}
			var plan model.ResolvedPlan
			if err := json.Unmarshal(data, &plan); err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"checkout": "example.com/architecture/" + tc.checkout, "payment": "example.com/architecture/" + tc.payment}
			if len(plan.Targets) != len(want) {
				t.Fatalf("unexpected targets: %s", data)
			}
			for _, target := range plan.Targets {
				if want[target.SpanName] != string(target.SymbolID) || target.RuleID != target.SpanName {
					t.Fatalf("unexpected target: %+v", target)
				}
				delete(want, target.SpanName)
			}
			invoke(t, root, 0, "lock")
			lockPath := filepath.Join(root, "otelplan.lock")
			original[lockPath], err = os.ReadFile(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			invoke(t, root, 0, "validate", "--strict")
			invoke(t, root, 0, "compile", "--output", filepath.Join(t.TempDir(), "generated"))
			binary := filepath.Join(t.TempDir(), "app")
			invoke(t, root, 0, "build", "--", "-race", "-buildvcs=false", "-o", binary, ".")
			output, err := exec.CommandContext(t.Context(), binary).Output()
			if err != nil {
				t.Fatal(err)
			}
			checkArchitectureTrace(t, output)
			invoke(t, root, 0, "lock", "--check")

			if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !entry.IsDir() {
					if _, exists := original[path]; !exists {
						t.Errorf("unexpected project file %s", path)
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			for path, want := range original {
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("changed project file %s", path)
				}
			}
		})
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
	if err := json.Unmarshal(output, &result); err != nil {
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
