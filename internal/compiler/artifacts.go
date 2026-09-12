package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

// StageArtifacts creates a private, fresh directory under parent. The caller
// owns its lifetime and must remove it when compilation or building is finished.
func StageArtifacts(parent string, files []otelc.GeneratedFile) (model.Artifacts, error) {
	artifacts := model.Artifacts{Files: make([]model.ArtifactFile, 0, len(files))}
	seen := map[string]bool{}
	for _, file := range files {
		if !artifactPath(file.Path) || seen[file.Path] {
			return model.Artifacts{}, fmt.Errorf("invalid or duplicate artifact path")
		}
		seen[file.Path] = true
		artifacts.Files = append(artifacts.Files, model.ArtifactFile{Path: file.Path, Digest: artifactDigest(file.Data)})
	}
	sort.Slice(artifacts.Files, func(i, j int) bool { return artifacts.Files[i].Path < artifacts.Files[j].Path })
	dir, err := os.MkdirTemp(parent, "otelplan-artifacts-")
	if err != nil {
		return model.Artifacts{}, fmt.Errorf("create artifact directory: %w", err)
	}
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.RemoveAll(dir)
		}
	}()
	for _, file := range files {
		filename := filepath.Join(dir, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			return model.Artifacts{}, fmt.Errorf("create artifact subdirectory: %w", err)
		}
		if err := os.WriteFile(filename, file.Data, 0600); err != nil {
			return model.Artifacts{}, fmt.Errorf("write artifact: %w", err)
		}
	}
	artifacts.Dir = dir
	succeeded = true
	return artifacts, nil
}

func artifactPath(path string) bool {
	return fs.ValidPath(path) && path != "." && filepath.IsLocal(path) && !strings.ContainsAny(path, "\\:")
}

func artifactDigest(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }

// VerifyArtifacts checks the complete file set, rejecting links and non-regular
// files. Expected hashes must come from trusted compilation or lock state.
func VerifyArtifacts(artifacts model.Artifacts) error {
	expected := map[string]string{}
	for _, file := range artifacts.Files {
		if !artifactPath(file.Path) || expected[file.Path] != "" || !validArtifactDigest(file.Digest) {
			return fmt.Errorf("invalid artifact inventory")
		}
		expected[file.Path] = file.Digest
	}
	root, err := os.OpenRoot(artifacts.Dir)
	if err != nil {
		return fmt.Errorf("open artifact directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	count := 0
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("artifact is not a regular file")
		}
		digest, ok := expected[path]
		if !ok {
			return fmt.Errorf("unexpected artifact file")
		}
		data, err := root.ReadFile(path)
		if err != nil {
			return err
		}
		if artifactDigest(data) != digest {
			return fmt.Errorf("artifact digest mismatch")
		}
		count++
		return nil
	})
	if err != nil {
		return fmt.Errorf("verify artifacts: %w", err)
	}
	if count != len(expected) {
		return fmt.Errorf("artifact file is missing")
	}
	return nil
}

func validArtifactDigest(value string) bool {
	digest, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && len(digest) == sha256.Size && value == fmt.Sprintf("sha256:%x", digest)
}
