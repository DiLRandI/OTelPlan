package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const (
	artifactStagingDirectoryMode = 0o700
	artifactStagingFileMode      = 0o600
)

var (
	errInvalidArtifactPath          = errors.New("invalid or duplicate artifact path")
	errInvalidArtifactInventory     = errors.New("invalid artifact inventory")
	errNonregularStagedArtifact     = errors.New("artifact is not a regular file")
	errUnexpectedStagedArtifact     = errors.New("unexpected artifact file")
	errStagedArtifactDigestMismatch = errors.New("artifact digest mismatch")
	errMissingStagedArtifact        = errors.New("artifact file is missing")
)

// StageArtifacts creates a private, fresh directory under parent. The caller
// owns its lifetime and must remove it when compilation or building is finished.
func StageArtifacts(parent string, files []otelc.GeneratedFile) (model.Artifacts, error) {
	var empty model.Artifacts

	inventory, err := generatedArtifactInventory(files)
	if err != nil {
		return empty, err
	}

	dir, err := os.MkdirTemp(parent, "otelplan-artifacts-")
	if err != nil {
		return empty, fmt.Errorf("create artifact directory: %w", err)
	}

	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.RemoveAll(dir)
		}
	}()

	root, err := os.OpenRoot(dir)
	if err != nil {
		return empty, fmt.Errorf("open fresh artifact directory: %w", err)
	}

	defer func() { _ = root.Close() }()

	err = writeGeneratedArtifacts(root, files)
	if err != nil {
		return empty, err
	}

	succeeded = true

	return model.Artifacts{Dir: dir, Files: inventory}, nil
}

func generatedArtifactInventory(files []otelc.GeneratedFile) ([]model.ArtifactFile, error) {
	inventory := make([]model.ArtifactFile, 0, len(files))
	seen := make(map[string]bool, len(files))

	for _, file := range files {
		if !artifactPath(file.Path) || seen[file.Path] {
			return nil, errInvalidArtifactPath
		}

		seen[file.Path] = true
		inventory = append(inventory, model.ArtifactFile{Path: file.Path, Digest: artifactDigest(file.Data)})
	}

	sort.Slice(inventory, func(i, j int) bool { return inventory[i].Path < inventory[j].Path })

	return inventory, nil
}

func writeGeneratedArtifacts(root *os.Root, files []otelc.GeneratedFile) error {
	for _, file := range files {
		name := filepath.FromSlash(file.Path)

		err := root.MkdirAll(filepath.Dir(name), artifactStagingDirectoryMode)
		if err != nil {
			return fmt.Errorf("create artifact subdirectory: %w", err)
		}

		err = root.WriteFile(name, file.Data, artifactStagingFileMode)
		if err != nil {
			return fmt.Errorf("write artifact: %w", err)
		}
	}

	return nil
}

func artifactPath(path string) bool {
	return fs.ValidPath(path) && path != "." && filepath.IsLocal(path) && !strings.ContainsAny(path, "\\:")
}

func artifactDigest(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }

// VerifyArtifacts checks the complete file set, rejecting links and non-regular
// files. Expected hashes must come from trusted compilation or lock state.
func VerifyArtifacts(artifacts model.Artifacts) error {
	expected, err := expectedArtifactInventory(artifacts.Files)
	if err != nil {
		return err
	}

	root, err := os.OpenRoot(artifacts.Dir)
	if err != nil {
		return fmt.Errorf("open artifact directory: %w", err)
	}

	defer func() { _ = root.Close() }()

	count := 0

	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("read artifact entry: %w", walkErr)
		}

		if entry.IsDir() {
			return nil
		}

		err := verifyStagedArtifactFile(root, expected, path, entry.Type())
		if err != nil {
			return err
		}

		count++

		return nil
	})
	if err != nil {
		return fmt.Errorf("verify artifacts: %w", err)
	}

	if count != len(expected) {
		return errMissingStagedArtifact
	}

	return nil
}

func expectedArtifactInventory(files []model.ArtifactFile) (map[string]string, error) {
	expected := make(map[string]string, len(files))
	for _, file := range files {
		if !artifactPath(file.Path) || expected[file.Path] != "" || !validArtifactDigest(file.Digest) {
			return nil, errInvalidArtifactInventory
		}

		expected[file.Path] = file.Digest
	}

	return expected, nil
}

func verifyStagedArtifactFile(root *os.Root, expected map[string]string, path string, mode fs.FileMode) error {
	if !mode.IsRegular() {
		return errNonregularStagedArtifact
	}

	digest, exists := expected[path]
	if !exists {
		return errUnexpectedStagedArtifact
	}

	data, err := root.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read staged artifact contents: %w", err)
	}

	if artifactDigest(data) != digest {
		return errStagedArtifactDigestMismatch
	}

	return nil
}

func validArtifactDigest(value string) bool {
	digest, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))

	return err == nil && len(digest) == sha256.Size && value == fmt.Sprintf("sha256:%x", digest)
}
