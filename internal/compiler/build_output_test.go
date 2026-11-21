package compiler

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
)

func backendArtifactFixture(t *testing.T) *os.Root {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	err = root.WriteFile("app", []byte("binary"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = root.Chmod("app", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	return root
}

func closeOnCleanup(t *testing.T, closer io.Closer) {
	t.Helper()
	t.Cleanup(func() {
		err := closer.Close()
		if err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
}

func TestReadBuildArtifact(t *testing.T) {
	t.Parallel()

	root := backendArtifactFixture(t)
	dir, path := root.Name(), filepath.Join(root.Name(), "app")

	artifact, err := readBuildArtifact(dir, path, "app")
	if err != nil {
		t.Fatal(err)
	}

	expected := BuildArtifact{Dir: dir, File: path, Digest: artifactDigest([]byte("binary")), DefaultName: "app"}
	if artifact != expected {
		t.Fatalf("artifact=%+v; want %+v", artifact, expected)
	}

	for _, invalid := range []string{dir, filepath.Join(dir, "missing")} {
		_, err := readBuildArtifact(dir, invalid, "app")
		if err == nil {
			t.Fatal("accepted non-file output")
		}
	}

	err = root.Symlink("app", "link")
	if err != nil {
		t.Fatal(err)
	}

	_, err = readBuildArtifact(dir, filepath.Join(dir, "link"), "app")
	if err == nil {
		t.Fatal("accepted symlink output")
	}
}

func TestReadBuildArtifactRejectsOutsideDirectory(t *testing.T) {
	t.Parallel()

	owned, outside := backendArtifactFixture(t), backendArtifactFixture(t)

	_, err := readBuildArtifact(owned.Name(), filepath.Join(outside.Name(), "app"), "app")
	if err == nil {
		t.Fatal("accepted an artifact outside its recorded directory")
	}
}

func TestReadOpenedArtifactRejectsDifferentFileIdentity(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	closeOnCleanup(t, root)

	contents := []byte("identical contents")
	for _, name := range []string{"expected", "opened"} {
		err = root.WriteFile(name, contents, 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	expected, err := root.Lstat("expected")
	if err != nil {
		t.Fatal(err)
	}

	opened, err := root.Open("opened")
	if err != nil {
		t.Fatal(err)
	}

	closeOnCleanup(t, opened)

	openedInfo, err := opened.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if os.SameFile(expected, openedInfo) {
		t.Fatal("test files unexpectedly share identity")
	}

	data, err := readOpenedArtifact(opened, expected)
	if data != nil {
		t.Fatalf("read data from a different file: %q", data)
	}

	if !errors.Is(err, errArtifactFileChanged) {
		t.Fatalf("different file identity error = %v; want %v", err, errArtifactFileChanged)
	}
}

func TestReadOpenedArtifactRejectsNonregularHandle(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	closeOnCleanup(t, root)

	err = root.WriteFile("expected", []byte("contents"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = root.Mkdir("directory", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	expected, err := root.Lstat("expected")
	if err != nil {
		t.Fatal(err)
	}

	opened, err := root.Open("directory")
	if err != nil {
		t.Fatal(err)
	}

	closeOnCleanup(t, opened)

	data, err := readOpenedArtifact(opened, expected)
	if data != nil {
		t.Fatalf("read data from a non-regular handle: %q", data)
	}

	if !errors.Is(err, errNonregularStagedArtifact) {
		t.Fatalf("non-regular handle error = %v; want %v", err, errNonregularStagedArtifact)
	}
}

func TestArtifactReplacementPreservesUnverifiedFiles(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()

	previous, err := StageArtifacts(parent, []otelc.GeneratedFile{
		{Path: "hooks.go", Data: []byte("package hooks")},
		{Path: "manifest.json", Data: []byte(`{
			"apiVersion": "otelplan.io/artifacts/v1alpha1",
			"files": [{"path": "hooks.go", "digest": "` + artifactDigest([]byte("package hooks")) + `"}]
		}`)},
	})
	if err != nil {
		t.Fatal(err)
	}

	previous, err = ReadArtifacts(previous.Dir)
	if err != nil {
		t.Fatal(err)
	}

	root, err := os.OpenRoot(previous.Dir)
	if err != nil {
		t.Fatal(err)
	}

	closeOnCleanup(t, root)

	const note = "keep this unrelated file"

	err = root.WriteFile("notes.txt", []byte(note), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	staged := t.TempDir()

	err = replaceArtifactOutput(previous, staged)
	if !errors.Is(err, errUnexpectedStagedArtifact) {
		t.Fatalf("replacement error = %v, want unexpected artifact error", err)
	}

	data, err := os.ReadFile(filepath.Join(previous.Dir, "notes.txt"))
	if err != nil || string(data) != note {
		t.Fatalf("previous output was not restored intact: %v, %q", err, data)
	}

	_, err = os.Stat(staged)
	if err != nil {
		t.Fatalf("replacement was committed despite failed verification: %v", err)
	}
}
