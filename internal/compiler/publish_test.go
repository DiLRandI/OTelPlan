package compiler

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestPublishArtifacts(t *testing.T) {
	backend, _ := otelc.Identity(otelc.SupportedVersion)
	bundle := func(version string) []otelc.GeneratedFile {
		t.Helper()
		files, err := otelc.RenderBundle(backend, version, &model.CodeModel{}, model.ResolvedPlan{}, "example.com/generated")
		if err != nil {
			t.Fatal(err)
		}
		return files
	}
	first, second := bundle("first"), bundle("second")
	destination := filepath.Join(t.TempDir(), "build")
	published, err := PublishArtifacts(destination, first, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyArtifacts(published); err != nil {
		t.Fatal(err)
	}
	repeated, err := PublishArtifacts(destination, first, false)
	if err != nil || !reflect.DeepEqual(published, repeated) {
		t.Fatal("identical output was not reused")
	}
	if _, err := PublishArtifacts(destination, second, false); err == nil {
		t.Fatal("replaced without clean")
	}
	if err := VerifyArtifacts(published); err != nil {
		t.Fatal("failed publish changed existing output")
	}
	replaced, err := PublishArtifacts(destination, second, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyArtifacts(replaced); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadArtifacts(destination)
	if err != nil || !reflect.DeepEqual(loaded, replaced) {
		t.Fatal("published inventory cannot be read")
	}
	entries, err := os.ReadDir(filepath.Dir(destination))
	if err != nil || len(entries) != 1 {
		t.Fatal("publication left temporary directories")
	}
	for _, change := range []struct {
		name   string
		modify func(string) error
	}{
		{"extra", func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "application.go"), []byte("package app"), 0600)
		}},
		{"modified", func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "hooks", "hooks.go"), []byte("changed"), 0600)
		}},
		{"malformed", func(dir string) error { return os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{}"), 0600) }},
		{"symlink", func(dir string) error { return os.Symlink("manifest.json", filepath.Join(dir, "extra")) }},
	} {
		t.Run(change.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "build")
			if _, err := PublishArtifacts(dir, first, false); err != nil {
				t.Fatal(err)
			}
			if err := change.modify(dir); err != nil {
				t.Fatal(err)
			}
			if _, err := PublishArtifacts(dir, second, true); err == nil {
				t.Fatal("clean accepted unverified output")
			}
			if _, err := os.Stat(dir); err != nil {
				t.Fatal("existing output removed")
			}
		})
	}
}
