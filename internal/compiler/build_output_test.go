package compiler

import (
	"os"
	"path/filepath"
	"testing"
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
