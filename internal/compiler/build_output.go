package compiler

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	privateBuildOutputMode          = 0o600
	privateBuildOutputDirectoryMode = 0o700
	buildOutputOwnerExecute         = 0o100
)

var (
	errInvalidBuildArtifact       = errors.New("invalid build artifact identity")
	errNonregularBuildArtifact    = errors.New("build artifact must be a regular file")
	errEmptyBuildOutputPath       = errors.New("build output path must not be empty")
	errNonregularBuildDestination = errors.New("build output cannot replace a directory or symlink")
	errChangedBuildArtifactDigest = errors.New("build artifact digest changed before publication")
	errNonregularBackendOutput    = errors.New("backend output is not a regular file")
)

// PublishBuildArtifact verifies and copies a temporary build artifact before
// replacing destination. It does not remove the caller-owned build directory.
func PublishBuildArtifact(artifact BuildArtifact, destination string) error {
	if !validArtifactDigest(artifact.Digest) {
		return errInvalidBuildArtifact
	}

	relative, err := relativeBuildArtifact(artifact.Dir, artifact.File)
	if err != nil {
		return err
	}

	root, err := os.OpenRoot(artifact.Dir)
	if err != nil {
		return fmt.Errorf("open staged build directory: %w", err)
	}

	defer func() { _ = root.Close() }()

	source, mode, err := openRegularBuildArtifact(root, relative)
	if err != nil {
		return err
	}

	defer func() { _ = source.Close() }()

	destination, err = prepareBuildDestination(destination)
	if err != nil {
		return err
	}

	temporary, err := os.CreateTemp(filepath.Dir(destination), ".otelplan-output-")
	if err != nil {
		return fmt.Errorf("stage verified build output: %w", err)
	}

	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporary.Name())
	}()

	err = copyVerifiedBuildOutput(temporary, source, artifact.Digest, mode)
	if err != nil {
		return err
	}

	err = temporary.Close()
	if err != nil {
		return fmt.Errorf("close verified build output: %w", err)
	}

	err = os.Rename(temporary.Name(), destination)
	if err != nil {
		return fmt.Errorf("publish verified build output: %w", err)
	}

	return nil
}

func relativeBuildArtifact(directory, filename string) (string, error) {
	if !filepath.IsAbs(directory) || !filepath.IsAbs(filename) || !withinTree(directory, filename) {
		return "", errInvalidBuildArtifact
	}

	relative, err := filepath.Rel(directory, filename)
	if err != nil {
		return "", fmt.Errorf("locate staged build artifact: %w", err)
	}

	return relative, nil
}

func openRegularBuildArtifact(root *os.Root, relative string) (*os.File, os.FileMode, error) {
	info, err := root.Lstat(relative)
	if err != nil {
		return nil, 0, fmt.Errorf("inspect staged build artifact: %w", err)
	}

	if !info.Mode().IsRegular() {
		return nil, 0, errNonregularBuildArtifact
	}

	source, err := root.Open(relative)
	if err != nil {
		return nil, 0, fmt.Errorf("open staged build artifact: %w", err)
	}

	return source, info.Mode(), nil
}

func prepareBuildDestination(destination string) (string, error) {
	if destination == "" {
		return "", errEmptyBuildOutputPath
	}

	absolute, err := filepath.Abs(destination)
	if err != nil {
		return "", fmt.Errorf("resolve build output destination: %w", err)
	}

	existing, err := os.Lstat(absolute)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("inspect build output destination: %w", err)
	}

	if err == nil && !existing.Mode().IsRegular() {
		return "", errNonregularBuildDestination
	}

	err = os.MkdirAll(filepath.Dir(absolute), privateBuildOutputDirectoryMode)
	if err != nil {
		return "", fmt.Errorf("create build output directory: %w", err)
	}

	return absolute, nil
}

func copyVerifiedBuildOutput(destination *os.File, source io.Reader, expectedDigest string, mode os.FileMode) error {
	digest := sha256.New()

	_, err := io.Copy(io.MultiWriter(destination, digest), source)
	if err != nil {
		return fmt.Errorf("copy staged build artifact: %w", err)
	}

	if fmt.Sprintf("sha256:%x", digest.Sum(nil)) != expectedDigest {
		return errChangedBuildArtifactDigest
	}

	err = destination.Chmod(privateBuildOutputMode | mode.Perm()&buildOutputOwnerExecute)
	if err != nil {
		return fmt.Errorf("set private build output permissions: %w", err)
	}

	return nil
}

func readBuildArtifact(directory, filename, name string) (BuildArtifact, error) {
	var empty BuildArtifact

	relative, err := relativeBuildArtifact(directory, filename)
	if err != nil {
		return empty, err
	}

	root, err := os.OpenRoot(directory)
	if err != nil {
		return empty, fmt.Errorf("open backend output directory: %w", err)
	}

	defer func() { _ = root.Close() }()

	file, _, err := openRegularBuildArtifact(root, relative)
	if errors.Is(err, errNonregularBuildArtifact) {
		return empty, errNonregularBackendOutput
	}

	if err != nil {
		return empty, fmt.Errorf("read backend output: %w", err)
	}

	defer func() { _ = file.Close() }()

	digest := sha256.New()

	_, err = io.Copy(digest, file)
	if err != nil {
		return empty, fmt.Errorf("hash backend output: %w", err)
	}

	return BuildArtifact{
		Dir: directory, File: filename, Digest: fmt.Sprintf("sha256:%x", digest.Sum(nil)), DefaultName: name,
	}, nil
}
