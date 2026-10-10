package otelc_test

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"reflect"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestBundleManifest(t *testing.T) {
	t.Parallel()

	code, plan := bundleFixture()

	for targetIndex := range plan.Targets {
		plan.Targets[targetIndex].SpanName = "operation"

		var attribute model.AttributePlan

		attribute.Key = "component"
		attribute.From.Constant = "app"
		plan.Targets[targetIndex].Attributes = []model.AttributePlan{attribute}
	}

	backend, err := otelc.Identity(otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	backend.Digest = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("backend")))

	files, err := otelc.RenderBundle(backend, "test", code, plan, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}

	manifest, payload := inspectBundleFiles(t, files)
	if !hasExpectedManifestIdentity(manifest, backend, len(payload)) {
		t.Fatalf("incomplete manifest: %+v", manifest)
	}

	assertManifestInventory(t, manifest, payload)

	plan.Targets[0], plan.Targets[2] = plan.Targets[2], plan.Targets[0]

	repeated, err := otelc.RenderBundle(backend, "test", code, plan, "example.com/generated")

	if err != nil || !reflect.DeepEqual(files, repeated) {
		t.Fatal("target reordering changed bundle")
	}

	changed, err := otelc.RenderBundle(backend, "next", code, plan, "example.com/generated")
	if err != nil || reflect.DeepEqual(files, changed) {
		t.Fatal("runtime version did not change bundle")
	}
}

func inspectBundleFiles(t *testing.T, files []otelc.GeneratedFile) (otelc.BundleManifest, map[string][]byte) {
	t.Helper()

	var manifest otelc.BundleManifest

	payload := make(map[string][]byte)

	previous := ""

	for _, file := range files {
		if file.Path <= previous {
			t.Fatal("unsorted or duplicate file")
		}

		previous = file.Path

		if file.Path == "manifest.json" {
			err := json.Unmarshal(file.Data, &manifest)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			payload[file.Path] = file.Data
		}

		if strings.HasSuffix(file.Path, ".go") {
			formatted, err := format.Source(file.Data)
			if err != nil || string(formatted) != string(file.Data) {
				t.Fatalf("invalid Go file %s: %v", file.Path, err)
			}
		}
	}

	return manifest, payload
}

func hasExpectedManifestIdentity(manifest otelc.BundleManifest, backend model.LockBackend, payloadCount int) bool {
	return manifest.Backend == backend && manifest.RuntimeVersion == "test" &&
		manifest.ModulePath == "example.com/generated" && len(manifest.Files) == payloadCount
}

func assertManifestInventory(t *testing.T, manifest otelc.BundleManifest, payload map[string][]byte) {
	t.Helper()

	for _, entry := range manifest.Files {
		data, ok := payload[entry.Path]
		if !ok || entry.Digest != fmt.Sprintf("sha256:%x", sha256.Sum256(data)) {
			t.Fatalf("incorrect inventory: %+v", entry)
		}
	}
}

func TestBundleRepeatedInvalidInputPreservesErrorIdentity(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name         string
		change       func(*model.LockBackend) string
		errorMessage string
	}{
		{name: "identity", change: func(backend *model.LockBackend) string {
			backend.Name = "other"

			return "example.com/generated"
		}, errorMessage: "backend identity does not match pinned capabilities"},
		{name: "capabilities", change: func(backend *model.LockBackend) string {
			backend.Capabilities.BeforeHook = false

			return "example.com/generated"
		}, errorMessage: "backend identity does not match pinned capabilities"},
		{name: "digest", change: func(backend *model.LockBackend) string {
			backend.Digest = "sha256:broken"

			return "example.com/generated"
		}, errorMessage: "invalid backend executable digest"},
		{name: "module path", change: func(_ *model.LockBackend) string {
			return "../escape"
		}, errorMessage: "invalid generated module path"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			backend := bundleIdentity(t)

			modulePath := testCase.change(&backend)

			var firstPlan, secondPlan model.ResolvedPlan

			_, firstErr := otelc.RenderBundle(backend, "test", new(model.CodeModel), firstPlan, modulePath)
			_, secondErr := otelc.RenderBundle(backend, "test", new(model.CodeModel), secondPlan, modulePath)

			if repeatedValidationFailed(firstErr, secondErr, testCase.errorMessage) {
				t.Fatalf("repeated validation did not preserve error identity: first=%v second=%v", firstErr, secondErr)
			}
		})
	}
}

func repeatedValidationFailed(firstErr, secondErr error, expectedMessage string) bool {
	return firstErr == nil || secondErr == nil || firstErr.Error() != expectedMessage || !errors.Is(secondErr, firstErr)
}

func TestBundleRejectsInvalidIdentity(t *testing.T) {
	t.Parallel()

	for _, change := range []func(*model.LockBackend){
		func(backend *model.LockBackend) { backend.Name = "other" },
		func(backend *model.LockBackend) { backend.Version = "v0.0.0" },
		func(backend *model.LockBackend) { backend.Capabilities.BeforeHook = false },
		func(backend *model.LockBackend) { backend.Digest = "sha256:broken" },
	} {
		backend := bundleIdentity(t)

		change(&backend)

		var plan model.ResolvedPlan

		files, err := otelc.RenderBundle(backend, "test", new(model.CodeModel), plan, "example.com/generated")
		if err == nil || files != nil {
			t.Fatal("invalid identity returned output")
		}
	}
}

func TestBundleWithoutTargets(t *testing.T) {
	t.Parallel()

	backend := bundleIdentity(t)

	var plan model.ResolvedPlan

	files, err := otelc.RenderBundle(backend, "test", new(model.CodeModel), plan, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 5 {
		t.Fatalf("unexpected empty bundle: %+v", files)
	}

	for _, file := range files {
		if strings.HasPrefix(file.Path, "rules/") && !strings.HasSuffix(file.Path, ".otelc.yaml") {
			t.Fatalf("backend cannot discover %s", file.Path)
		}
	}

	for _, modulePath := range []string{"../escape", "example.com/generated/hooks/.."} {
		files, err := otelc.RenderBundle(backend, "test", new(model.CodeModel), plan, modulePath)
		if err == nil || files != nil {
			t.Fatal("invalid module path returned output")
		}
	}
}

func bundleIdentity(t *testing.T) model.LockBackend {
	t.Helper()

	backend, err := otelc.Identity(otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	return backend
}

func bundleFixture() (*model.CodeModel, model.ResolvedPlan) {
	code := new(model.CodeModel)

	function := bundleSymbol("example.com/app.Run", model.SymbolFunction, "Run")
	pointerMethod := bundleSymbol("example.com/app.(*Worker).Run", model.SymbolMethod, "Run")
	pointerReceiver := new(model.Receiver)
	pointerReceiver.Type = "Worker"
	pointerReceiver.Pointer = true
	pointerMethod.Receiver = pointerReceiver
	valueMethod := bundleSymbol("example.com/app.(Worker).Other", model.SymbolMethod, "Other")
	valueReceiver := new(model.Receiver)
	valueReceiver.Type = "Worker"
	valueMethod.Receiver = valueReceiver
	code.Symbols = []model.Symbol{function, pointerMethod, valueMethod}

	var plan model.ResolvedPlan

	for _, symbol := range code.Symbols {
		var target model.ResolvedTarget

		target.SymbolID = symbol.ID
		target.Signature = symbol.Signature
		target.RuleID = "chosen"
		target.ContextStrategy.Strategy = model.ContextStrategyRoot
		plan.Targets = append(plan.Targets, target)
	}

	return code, plan
}

func bundleSymbol(id string, kind model.SymbolKind, name string) model.Symbol {
	var symbol model.Symbol

	symbol.ID = model.SymbolID(id)
	symbol.Kind = kind
	symbol.PackageImportPath = "example.com/app"
	symbol.PackageName = "app"
	symbol.Name = name
	symbol.HasBody = true
	symbol.Signature = "func()"

	return symbol
}
