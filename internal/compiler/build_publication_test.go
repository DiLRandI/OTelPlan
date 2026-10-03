package compiler_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
)

const publicationFixtureContents = "built output"

func publicationFixture(t *testing.T) (compiler.BuildArtifact, *os.Root) {
	t.Helper()

	directory := t.TempDir()

	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	err = root.WriteFile("output", []byte(publicationFixtureContents), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = root.Chmod("output", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	return compiler.BuildArtifact{
		Dir: directory, File: filepath.Join(directory, "output"),
		Digest: fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(publicationFixtureContents))), DefaultName: "app",
	}, root
}

func TestPublishBuildArtifact(t *testing.T) {
	t.Parallel()

	artifact, staged := publicationFixture(t)
	parent := t.TempDir()

	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = compiler.PublishBuildArtifact(artifact, filepath.Join(parent, "bin", "app"))
	if err != nil {
		t.Fatal(err)
	}

	assertPublicationContents(t, root, "bin/app", publicationFixtureContents)

	info, err := root.Stat("bin/app")
	if err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatal("executable permissions were not preserved privately")
	}

	assertPublicationContents(t, staged, "output", publicationFixtureContents)

	entries, err := fs.ReadDir(root.FS(), "bin")
	if err != nil || len(entries) != 1 {
		t.Fatal("publication left temporary files", err)
	}
}

func TestFailedPublicationPreservesExistingOutput(t *testing.T) {
	t.Parallel()

	artifact, _ := publicationFixture(t)
	parent := t.TempDir()

	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = root.WriteFile("app", []byte("previous"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	invalid := artifact
	invalid.Digest = "sha256:" + strings.Repeat("0", sha256.Size*2)

	err = compiler.PublishBuildArtifact(invalid, filepath.Join(parent, "app"))
	if err == nil {
		t.Fatal("accepted changed artifact")
	}

	previous, err := root.ReadFile("app")
	if err != nil || string(previous) != "previous" {
		t.Fatal("failed publication replaced previous output", err)
	}

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil || len(entries) != 1 {
		t.Fatal("failed publication left temporary files", err)
	}

	err = compiler.PublishBuildArtifact(artifact, filepath.Join(parent, "app"))
	if err != nil {
		t.Fatal(err)
	}
}

func TestPublicationRejectsDirectoryAndSymlinkDestinations(t *testing.T) {
	t.Parallel()

	artifact, _ := publicationFixture(t)
	parent := t.TempDir()

	err := compiler.PublishBuildArtifact(artifact, parent)
	if err == nil {
		t.Fatal("replaced output directory")
	}

	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = root.Symlink(artifact.File, "link")
	if err != nil {
		t.Fatal(err)
	}

	err = compiler.PublishBuildArtifact(artifact, filepath.Join(parent, "link"))
	if err == nil {
		t.Fatal("replaced output symlink")
	}
}

func TestPublicationRejectsSymlinkArtifacts(t *testing.T) {
	t.Parallel()

	artifact, root := publicationFixture(t)

	err := root.Symlink("output", "link")
	if err != nil {
		t.Fatal(err)
	}

	artifact.File = filepath.Join(artifact.Dir, "link")

	err = compiler.PublishBuildArtifact(artifact, filepath.Join(t.TempDir(), "app"))
	if err == nil {
		t.Fatal("accepted symlink artifact")
	}
}

func TestPublicationMissingArtifactPreservesCause(t *testing.T) {
	t.Parallel()

	artifact, _ := publicationFixture(t)
	artifact.File = filepath.Join(artifact.Dir, "missing")

	err := compiler.PublishBuildArtifact(artifact, filepath.Join(t.TempDir(), "app"))
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "inspect staged build artifact") {
		t.Fatal("missing staged artifact lost its filesystem cause or operation context", err)
	}
}

func TestPublicationIdentityErrorsShareCause(t *testing.T) {
	t.Parallel()

	artifact := new(compiler.BuildArtifact)
	first := compiler.PublishBuildArtifact(*artifact, "")

	second := compiler.PublishBuildArtifact(*artifact, "anything")
	if first == nil || !errors.Is(second, first) || first.Error() != "invalid build artifact identity" {
		t.Fatal("invalid artifact identities lost their shared cause or meaningful message")
	}
}

func assertPublicationContents(t *testing.T, root *os.Root, path, expected string) {
	t.Helper()

	contents, err := root.ReadFile(path)
	if err != nil || string(contents) != expected {
		t.Fatal("publication content changed unexpectedly", err)
	}
}

func TestStagedArtifactMutationPreventsPublication(t *testing.T) {
	t.Parallel()

	artifact, staged := publicationFixture(t)

	err := staged.WriteFile("output", []byte("changed staged output"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	parent := t.TempDir()

	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = compiler.PublishBuildArtifact(artifact, filepath.Join(parent, "app"))
	if err == nil {
		t.Fatal("published staged bytes that no longer match the verified digest")
	}

	_, err = root.Lstat("app")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("failed verification left a published destination", err)
	}
}
