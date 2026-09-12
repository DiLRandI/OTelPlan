package otelc

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/format"
	"reflect"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestBundleManifest(t *testing.T) {
	code, plan := ruleFixture()
	for i := range plan.Targets {
		plan.Targets[i].SpanName = "operation"
		plan.Targets[i].Attributes = []model.AttributePlan{{Key: "component", From: model.AttributeSource{Constant: "app"}}}
	}
	backend, _ := Identity(SupportedVersion)
	backend.Digest = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("backend")))
	files, err := RenderBundle(backend, "test", code, plan, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}
	var manifest BundleManifest
	payload := map[string][]byte{}
	previous := ""
	for _, file := range files {
		if file.Path <= previous {
			t.Fatal("unsorted or duplicate file")
		}
		previous = file.Path
		if file.Path == "manifest.json" {
			if err := json.Unmarshal(file.Data, &manifest); err != nil {
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
	if manifest.Backend != backend || manifest.RuntimeVersion != "test" || manifest.ModulePath != "example.com/generated" || len(manifest.Files) != len(payload) {
		t.Fatalf("incomplete manifest: %+v", manifest)
	}
	for _, entry := range manifest.Files {
		data, ok := payload[entry.Path]
		if !ok || entry.Digest != fmt.Sprintf("sha256:%x", sha256.Sum256(data)) {
			t.Fatalf("incorrect inventory: %+v", entry)
		}
	}
	plan.Targets[0], plan.Targets[2] = plan.Targets[2], plan.Targets[0]
	repeated, err := RenderBundle(backend, "test", code, plan, "example.com/generated")
	if err != nil || !reflect.DeepEqual(files, repeated) {
		t.Fatal("target reordering changed bundle")
	}
	changed, err := RenderBundle(backend, "next", code, plan, "example.com/generated")
	if err != nil || reflect.DeepEqual(files, changed) {
		t.Fatal("runtime version did not change bundle")
	}
}

func TestBundleRejectsInvalidIdentity(t *testing.T) {
	for _, change := range []func(*model.LockBackend){
		func(b *model.LockBackend) { b.Name = "other" },
		func(b *model.LockBackend) { b.Version = "v0.0.0" },
		func(b *model.LockBackend) { b.Capabilities.BeforeHook = false },
		func(b *model.LockBackend) { b.Digest = "sha256:broken" },
	} {
		backend, _ := Identity(SupportedVersion)
		change(&backend)
		files, err := RenderBundle(backend, "test", &model.CodeModel{}, model.ResolvedPlan{}, "example.com/generated")
		if err == nil || files != nil {
			t.Fatal("invalid identity returned output")
		}
	}
}

func TestBundleWithoutTargets(t *testing.T) {
	backend, _ := Identity(SupportedVersion)
	files, err := RenderBundle(backend, "test", &model.CodeModel{}, model.ResolvedPlan{}, "example.com/generated")
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
		files, err := RenderBundle(backend, "test", &model.CodeModel{}, model.ResolvedPlan{}, modulePath)
		if err == nil || files != nil {
			t.Fatal("invalid module path returned output")
		}
	}
}
