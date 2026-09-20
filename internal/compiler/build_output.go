package compiler

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// PublishBuildArtifact verifies and copies a temporary build artifact before
// replacing destination. It does not remove the caller-owned build directory.
func PublishBuildArtifact(artifact BuildArtifact, destination string) error {
	if !validArtifactDigest(artifact.Digest) || !filepath.IsAbs(artifact.Dir) || !filepath.IsAbs(artifact.File) || !withinTree(artifact.Dir, artifact.File) {
		return errors.New("invalid build artifact identity")
	}

	root, err := os.OpenRoot(artifact.Dir)
	if err != nil {
		return err
	}

	defer func() { _ = root.Close() }()

	relative, err := filepath.Rel(artifact.Dir, artifact.File)
	if err != nil {
		return err
	}

	info, err := root.Lstat(relative)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("build artifact must be a regular file")
	}

	source, err := root.Open(relative)
	if err != nil {
		return err
	}

	defer func() { _ = source.Close() }()

	if destination == "" {
		return errors.New("build output path must not be empty")
	}

	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}

	if existing, err := os.Lstat(destination); err == nil {
		if !existing.Mode().IsRegular() {
			return errors.New("build output cannot replace a directory or symlink")
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(filepath.Dir(destination), ".otelplan-output-")
	if err != nil {
		return err
	}

	defer func() { _ = temporary.Close(); _ = os.Remove(temporary.Name()) }()

	digest := sha256.New()
	if _, err := io.Copy(io.MultiWriter(temporary, digest), source); err != nil {
		return err
	}

	if fmt.Sprintf("sha256:%x", digest.Sum(nil)) != artifact.Digest {
		return errors.New("build artifact digest changed before publication")
	}

	if err := temporary.Chmod(0o600 | info.Mode().Perm()&0o100); err != nil {
		return err
	}

	if err := temporary.Close(); err != nil {
		return err
	}

	return os.Rename(temporary.Name(), destination)
}

func readBuildArtifact(dir, path, name string) (BuildArtifact, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return BuildArtifact{}, errors.New("backend output is not a regular file")
	}

	file, err := os.Open(path)
	if err != nil {
		return BuildArtifact{}, err
	}

	defer func() { _ = file.Close() }()

	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return BuildArtifact{}, err
	}

	return BuildArtifact{Dir: dir, File: path, Digest: fmt.Sprintf("sha256:%x", digest.Sum(nil)), DefaultName: name}, nil
}
