package compiler_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func stagedFixtureFiles() []otelc.GeneratedFile {
	return []otelc.GeneratedFile{
		{Path: "hooks/hooks.go", Data: []byte("package hooks\n")},
		{Path: "manifest.json", Data: []byte("{}\n")},
	}
}

func TestStageAndVerifyArtifacts(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	files := stagedFixtureFiles()

	first, err := compiler.StageArtifacts(parent, files)
	if err != nil {
		t.Fatal(err)
	}

	second, err := compiler.StageArtifacts(parent, files)
	if err != nil {
		t.Fatal(err)
	}

	if first.Dir == second.Dir || !slices.Equal(first.Files, second.Files) {
		t.Fatal("staging changed identity or reused directory")
	}

	err = compiler.VerifyArtifacts(first)
	if err != nil {
		t.Fatal(err)
	}

	err = compiler.VerifyArtifacts(second)
	if err != nil {
		t.Fatal("independent staged bundle changed", err)
	}
}

func TestArtifactVerificationRejectsTampering(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	for _, testCase := range []struct {
		name   string
		mutate func(*os.Root) error
	}{
		{name: "modified", mutate: func(root *os.Root) error {
			return root.WriteFile("manifest.json", []byte("changed"), 0o600)
		}},
		{name: "missing", mutate: func(root *os.Root) error { return root.Remove("manifest.json") }},
		{name: "extra", mutate: func(root *os.Root) error { return root.WriteFile("extra", nil, 0o600) }},
		{name: "symlink", mutate: func(root *os.Root) error { return root.Symlink("manifest.json", "link") }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			staged, err := compiler.StageArtifacts(parent, stagedFixtureFiles())
			if err != nil {
				t.Fatal(err)
			}

			root, err := os.OpenRoot(staged.Dir)
			if err != nil {
				t.Fatal(err)
			}

			defer func() { _ = root.Close() }()

			err = testCase.mutate(root)
			if err != nil {
				t.Fatal(err)
			}

			err = compiler.VerifyArtifacts(staged)
			if err == nil {
				t.Fatal("accepted changed artifacts")
			}
		})
	}
}

func TestStageRejectsUnsafePathsWithoutWrites(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	for _, path := range []string{"../escape", "/absolute", "a/../b", "a\\b", "C:drive", "."} {
		_, err := compiler.StageArtifacts(parent, []otelc.GeneratedFile{{Path: path, Data: nil}})
		if err == nil {
			t.Fatalf("accepted %q", path)
		}
	}

	_, err := compiler.StageArtifacts(parent, []otelc.GeneratedFile{{Path: "same", Data: nil}, {Path: "same", Data: nil}})
	if err == nil {
		t.Fatal("accepted duplicate")
	}

	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("invalid input wrote files", err)
	}
}

func TestStageCleansPartialFailure(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	files := []otelc.GeneratedFile{{Path: "a", Data: []byte("file")}, {Path: "a/b", Data: nil}}

	_, err := compiler.StageArtifacts(parent, files)
	if err == nil {
		t.Fatal("accepted file-directory conflict")
	}

	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("failed staging left files", err)
	}
}

func generatedArtifactBundle(t *testing.T) model.Artifacts {
	t.Helper()

	backend, err := otelc.Identity(otelc.SupportedVersion)
	if err != nil {
		t.Fatal(err)
	}

	code, plan := new(model.CodeModel), new(model.ResolvedPlan)

	files, err := otelc.RenderBundle(backend, "test", code, *plan, "example.com/generated")
	if err != nil {
		t.Fatal(err)
	}

	staged, err := compiler.StageArtifacts(t.TempDir(), files)
	if err != nil {
		t.Fatal(err)
	}

	return staged
}

func TestStageGeneratedBundle(t *testing.T) {
	t.Parallel()

	staged := generatedArtifactBundle(t)

	err := compiler.VerifyArtifacts(staged)
	if err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(staged.Dir)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	foundManifest := false

	for _, file := range staged.Files {
		if file.Path == "manifest.json" {
			foundManifest = true
		}

		info, err := root.Stat(filepath.FromSlash(file.Path))
		if err != nil {
			t.Fatal(err)
		}

		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			t.Fatal("artifact readable outside owner")
		}
	}

	if !foundManifest {
		t.Fatal("manifest was not hashed")
	}
}

func TestArtifactVerificationRejectsMalformedDigests(t *testing.T) {
	t.Parallel()

	staged := generatedArtifactBundle(t)
	for _, digest := range []string{"", strings.Repeat("x", 71), "sha256:" + strings.Repeat("A", 64)} {
		changed := staged
		changed.Files = slices.Clone(staged.Files)
		changed.Files[0].Digest = digest

		err := compiler.VerifyArtifacts(changed)
		if err == nil {
			t.Fatal("accepted malformed digest")
		}
	}
}

func TestArtifactStagingInvalidPathSharesCause(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	files := []otelc.GeneratedFile{{Path: "../escape", Data: nil}}
	_, first := compiler.StageArtifacts(parent, files)

	_, second := compiler.StageArtifacts(parent, files)
	if first == nil || !errors.Is(second, first) || first.Error() != "invalid or duplicate artifact path" {
		t.Fatal("invalid artifact paths lost their stable cause or meaningful message")
	}
}

func TestArtifactVerificationInvalidInventorySharesCause(t *testing.T) {
	t.Parallel()

	staged := new(model.Artifacts)
	staged.Dir = t.TempDir()
	staged.Files = []model.ArtifactFile{{Path: "../escape", Digest: "invalid"}}
	first := compiler.VerifyArtifacts(*staged)

	second := compiler.VerifyArtifacts(*staged)
	if first == nil || !errors.Is(second, first) || first.Error() != "invalid artifact inventory" {
		t.Fatal("invalid artifact inventory lost its stable cause or meaningful message")
	}
}

func TestArtifactVerificationMissingDirectoryPreservesCause(t *testing.T) {
	t.Parallel()

	staged := new(model.Artifacts)
	staged.Dir = filepath.Join(t.TempDir(), "missing")

	err := compiler.VerifyArtifacts(*staged)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("missing artifact directory lost its filesystem cause", err)
	}
}
