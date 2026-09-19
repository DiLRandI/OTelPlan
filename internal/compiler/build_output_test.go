package compiler

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPublishBuildArtifact(t *testing.T) {
	source := t.TempDir()
	data := []byte("built output")
	path := filepath.Join(source, "output")
	if err := os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	artifact := BuildArtifact{Dir: source, File: path, Digest: artifactDigest(data)}
	parent := t.TempDir()
	destination := filepath.Join(parent, "bin", "app")
	if err := PublishBuildArtifact(artifact, destination); err != nil {
		t.Fatal(err)
	}
	published, err := os.ReadFile(destination)
	if err != nil || string(published) != string(data) {
		t.Fatal("wrong published output")
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0700 {
		t.Fatal("executable permissions were not preserved privately")
	}
	if err := os.WriteFile(destination, []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	invalid := artifact
	invalid.Digest = "sha256:" + strings.Repeat("0", 64)
	if err := PublishBuildArtifact(invalid, destination); err == nil {
		t.Fatal("accepted changed artifact")
	}
	published, err = os.ReadFile(destination)
	if err != nil || string(published) != "previous" {
		t.Fatal("failed publication replaced previous output")
	}
	if err := PublishBuildArtifact(artifact, destination); err != nil {
		t.Fatal(err)
	}
	if original, err := os.ReadFile(path); err != nil || string(original) != string(data) {
		t.Fatal("publication changed staged output")
	}
	entries, err := os.ReadDir(filepath.Dir(destination))
	if err != nil || len(entries) != 1 {
		t.Fatal("publication left temporary files")
	}
	if err := PublishBuildArtifact(artifact, parent); err == nil {
		t.Fatal("replaced output directory")
	}
	link := filepath.Join(parent, "link")
	if err := os.Symlink(destination, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := PublishBuildArtifact(artifact, link); err == nil {
		t.Fatal("replaced output symlink")
	}
	linkedArtifact := artifact
	linkedArtifact.File = filepath.Join(source, "link")
	if err := os.Symlink(path, linkedArtifact.File); err != nil {
		t.Fatal(err)
	}
	if err := PublishBuildArtifact(linkedArtifact, destination); err == nil {
		t.Fatal("accepted symlink artifact")
	}
}

func TestReadBuildArtifact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app")
	data := []byte("binary")
	if err := os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	artifact, err := readBuildArtifact(dir, path, "app")
	if err != nil || artifact.Dir != dir || artifact.File != path || artifact.Digest != artifactDigest(data) || artifact.DefaultName != "app" {
		t.Fatalf("artifact=%+v, %v", artifact, err)
	}
	for _, invalid := range []string{dir, filepath.Join(dir, "missing")} {
		if _, err := readBuildArtifact(dir, invalid, "app"); err == nil {
			t.Fatal("accepted non-file output")
		}
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := readBuildArtifact(dir, link, "app"); err == nil {
		t.Fatal("accepted symlink output")
	}
}
