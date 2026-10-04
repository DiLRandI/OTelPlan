package compiler_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func publicationBundle(t *testing.T, version string) []otelc.GeneratedFile {
	t.Helper()

	backend, err := otelc.Identity(otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	code, plan := new(model.CodeModel), new(model.ResolvedPlan)

	files, err := otelc.RenderBundle(backend, version, code, *plan, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}

	return files
}

func publishBundle(t *testing.T, destination, version string, clean bool) model.Artifacts {
	t.Helper()

	artifacts, err := compiler.PublishArtifacts(destination, publicationBundle(t, version), clean)
	if err != nil {
		t.Fatal(err)
	}

	err = compiler.VerifyArtifacts(artifacts)
	if err != nil {
		t.Fatal(err)
	}

	return artifacts
}

func TestPublishArtifactsReusesIdenticalOutput(t *testing.T) {
	t.Parallel()

	destination := filepath.Join(t.TempDir(), "build")
	published := publishBundle(t, destination, "first", false)

	repeated := publishBundle(t, destination, "first", false)
	if !reflect.DeepEqual(published, repeated) {
		t.Fatal("identical output was not reused")
	}
}

func TestPublishArtifactsRequiresCleanForReplacement(t *testing.T) {
	t.Parallel()

	destination := filepath.Join(t.TempDir(), "build")
	published := publishBundle(t, destination, "first", false)
	second := publicationBundle(t, "second")

	_, firstErr := compiler.PublishArtifacts(destination, second, false)

	_, secondErr := compiler.PublishArtifacts(destination, second, false)
	if firstErr == nil || !errors.Is(secondErr, firstErr) || !strings.Contains(firstErr.Error(), "use --clean") {
		t.Fatal("replacement refusal lost its stable cause or meaningful message", firstErr, secondErr)
	}

	err := compiler.VerifyArtifacts(published)
	if err != nil {
		t.Fatal("refused publication changed existing output", err)
	}
}

func TestPublishArtifactsReplacesVerifiedOutput(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	destination := filepath.Join(parent, "build")
	_ = publishBundle(t, destination, "first", false)
	replaced := publishBundle(t, destination, "second", true)

	loaded, err := compiler.ReadArtifacts(destination)
	if err != nil || !reflect.DeepEqual(loaded, replaced) {
		t.Fatal("published inventory cannot be read", err)
	}

	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 1 {
		t.Fatal("publication left temporary directories", err)
	}
}

func TestPublishArtifactsRejectsUnownedOutput(t *testing.T) {
	t.Parallel()

	for _, change := range []struct {
		name   string
		modify func(*os.Root) error
	}{
		{name: "extra", modify: func(root *os.Root) error {
			return root.WriteFile("application.go", []byte("package app"), 0o600)
		}},
		{name: "modified", modify: func(root *os.Root) error {
			return root.WriteFile(filepath.Join("hooks", "hooks.go"), []byte("changed"), 0o600)
		}},
		{name: "malformed", modify: func(root *os.Root) error {
			return root.WriteFile("manifest.json", []byte("{}"), 0o600)
		}},
		{name: "symlink", modify: func(root *os.Root) error {
			return root.Symlink("manifest.json", "extra")
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "build")
			_ = publishBundle(t, dir, "first", false)

			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}

			defer func() { _ = root.Close() }()

			err = change.modify(root)
			if err != nil {
				t.Fatal(err)
			}

			_, err = compiler.PublishArtifacts(dir, publicationBundle(t, "second"), true)
			if err == nil {
				t.Fatal("clean accepted unverified output")
			}

			_, err = os.Stat(dir)
			if err != nil {
				t.Fatal("existing output removed", err)
			}
		})
	}
}

func TestReadArtifactsPreservesFilesystemCauses(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name      string
		exists    bool
		operation string
	}{
		{name: "missing directory", exists: false, operation: "inspect artifact output"},
		{name: "missing manifest", exists: true, operation: "read artifact manifest"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if !testCase.exists {
				dir = filepath.Join(dir, "missing")
			}

			_, err := compiler.ReadArtifacts(dir)
			if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), testCase.operation) {
				t.Fatal("missing artifact input lost its cause or operation", err)
			}
		})
	}
}

func TestReadArtifactsPreservesJSONCause(t *testing.T) {
	t.Parallel()

	files := []otelc.GeneratedFile{{Path: "manifest.json", Data: []byte(`{"files":]}`)}}

	staged, err := compiler.StageArtifacts(t.TempDir(), files)
	if err != nil {
		t.Fatal(err)
	}

	_, err = compiler.ReadArtifacts(staged.Dir)

	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) || !strings.Contains(err.Error(), "invalid artifact manifest") {
		t.Fatal("malformed manifest lost its JSON cause or operation", err)
	}
}

func TestReadArtifactsRejectsUnsupportedManifest(t *testing.T) {
	t.Parallel()

	for _, manifest := range []string{
		`{}`, `{"apiVersion":"future","files":[]}`, `{"unknown":true}`, `{} {}`, `{} trailing`,
	} {
		t.Run(manifest, func(t *testing.T) {
			t.Parallel()

			files := []otelc.GeneratedFile{{Path: "manifest.json", Data: []byte(manifest)}}

			staged, err := compiler.StageArtifacts(t.TempDir(), files)
			if err != nil {
				t.Fatal(err)
			}

			_, err = compiler.ReadArtifacts(staged.Dir)
			if err == nil {
				t.Fatal("accepted unsupported manifest")
			}
		})
	}
}

func TestReadArtifactsRejectsTrailingJSON(t *testing.T) {
	t.Parallel()

	for _, suffix := range []string{" {}", " trailing"} {
		t.Run(suffix, func(t *testing.T) {
			t.Parallel()

			artifacts := generatedArtifactBundle(t)

			root, err := os.OpenRoot(artifacts.Dir)
			if err != nil {
				t.Fatal(err)
			}

			defer func() { _ = root.Close() }()

			manifest, err := root.ReadFile("manifest.json")
			if err != nil {
				t.Fatal(err)
			}

			manifest = append(manifest, suffix...)

			err = root.WriteFile("manifest.json", manifest, 0o600)
			if err != nil {
				t.Fatal(err)
			}

			_, err = compiler.ReadArtifacts(artifacts.Dir)
			if err == nil {
				t.Fatal("accepted trailing JSON after a valid manifest")
			}
		})
	}
}

func TestPublishArtifactsCleansInvalidStagedBundle(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	destination := filepath.Join(parent, "build")
	files := []otelc.GeneratedFile{{Path: "manifest.json", Data: []byte("{}")}}

	_, err := compiler.PublishArtifacts(destination, files, false)
	if err == nil {
		t.Fatal("published invalid staged manifest")
	}

	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("invalid staged publication left files", err)
	}
}

func TestPublishArtifactsRejectsSymlinkDestination(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	destination := filepath.Join(parent, "link")
	published := publishBundle(t, filepath.Join(parent, "build"), "first", false)

	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = root.Symlink("build", "link")
	if err != nil {
		t.Fatal(err)
	}

	_, err = compiler.PublishArtifacts(destination, publicationBundle(t, "second"), true)
	if err == nil {
		t.Fatal("clean accepted a symlink destination")
	}

	err = compiler.VerifyArtifacts(published)
	if err != nil {
		t.Fatal("symlink target was changed", err)
	}

	target, err := root.Readlink("link")
	if err != nil || target != "build" {
		t.Fatal("symlink was changed", err)
	}
}
