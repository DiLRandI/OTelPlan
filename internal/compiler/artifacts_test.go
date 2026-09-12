package compiler

import (
	"github.com/DiLRandI/OTelPlan/pkg/model"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
)

func TestStageAndVerifyArtifacts(t *testing.T) {
	parent := t.TempDir()
	files := []otelc.GeneratedFile{{Path: "hooks/hooks.go", Data: []byte("package hooks\n")}, {Path: "manifest.json", Data: []byte("{}\n")}}
	first, err := StageArtifacts(parent, files)
	if err != nil {
		t.Fatal(err)
	}
	second, err := StageArtifacts(parent, files)
	if err != nil {
		t.Fatal(err)
	}
	if first.Dir == second.Dir || !reflect.DeepEqual(first.Files, second.Files) {
		t.Fatal("staging changed identity or reused directory")
	}
	if err := VerifyArtifacts(first); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		name   string
		mutate func(string) error
	}{
		{"modified", func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("changed"), 0600)
		}},
		{"missing", func(dir string) error { return os.Remove(filepath.Join(dir, "manifest.json")) }},
		{"extra", func(dir string) error { return os.WriteFile(filepath.Join(dir, "extra"), nil, 0600) }},
		{"symlink", func(dir string) error {
			return os.Symlink(filepath.Join(second.Dir, "manifest.json"), filepath.Join(dir, "link"))
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			staged, err := StageArtifacts(parent, files)
			if err != nil {
				t.Fatal(err)
			}
			if err := change.mutate(staged.Dir); err != nil {
				t.Fatal(err)
			}
			if err := VerifyArtifacts(staged); err == nil {
				t.Fatal("accepted changed artifacts")
			}
		})
	}
	if err := VerifyArtifacts(second); err != nil {
		t.Fatalf("other staged bundle changed: %v", err)
	}
}

func TestStageRejectsUnsafePathsWithoutWrites(t *testing.T) {
	parent := t.TempDir()
	for _, path := range []string{"../escape", "/absolute", "a/../b", "a\\b", "C:drive", "."} {
		if _, err := StageArtifacts(parent, []otelc.GeneratedFile{{Path: path}}); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
	if _, err := StageArtifacts(parent, []otelc.GeneratedFile{{Path: "same"}, {Path: "same"}}); err == nil {
		t.Fatal("accepted duplicate")
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("invalid input wrote files")
	}
}

func TestStageCleansPartialFailure(t *testing.T) {
	parent := t.TempDir()
	_, err := StageArtifacts(parent, []otelc.GeneratedFile{{Path: "a", Data: []byte("file")}, {Path: "a/b"}})
	if err == nil {
		t.Fatal("accepted file-directory conflict")
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("failed staging left files")
	}
}

func TestStageGeneratedBundle(t *testing.T) {
	backend, err := otelc.Identity(otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}
	files, err := otelc.RenderBundle(backend, "test", &model.CodeModel{}, model.ResolvedPlan{}, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}
	staged, err := StageArtifacts(t.TempDir(), files)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyArtifacts(staged); err != nil {
		t.Fatal(err)
	}
	foundManifest := false
	for _, file := range staged.Files {
		if file.Path == "manifest.json" {
			foundManifest = true
		}
		info, err := os.Stat(filepath.Join(staged.Dir, filepath.FromSlash(file.Path)))
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
			t.Fatal("artifact readable outside owner")
		}
	}
	if !foundManifest {
		t.Fatal("manifest was not hashed")
	}
	for _, digest := range []string{"", strings.Repeat("x", 71), "sha256:" + strings.Repeat("A", 64)} {
		changed := staged
		changed.Files = append([]model.ArtifactFile(nil), staged.Files...)
		changed.Files[0].Digest = digest
		if err := VerifyArtifacts(changed); err == nil {
			t.Fatal("accepted malformed digest")
		}
	}
}
