package compiler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

var (
	errArtifactOutputNotDirectory  = errors.New("artifact output must be a directory")
	errUnsupportedArtifactManifest = errors.New("unsupported or invalid artifact manifest")
	errArtifactOutputDiffers       = errors.New("artifact output differs; use --clean to replace verified output")
)

// ReadArtifacts verifies a bundle against its own manifest. This establishes
// file ownership for refresh, not authenticity against a trusted lockfile.
func ReadArtifacts(dir string) (model.Artifacts, error) {
	var empty model.Artifacts

	root, err := openArtifactOutput(dir)
	if err != nil {
		return empty, err
	}

	defer func() { _ = root.Close() }()

	data, err := root.ReadFile("manifest.json")
	if err != nil {
		return empty, fmt.Errorf("read artifact manifest: %w", err)
	}

	files, err := artifactManifestInventory(data)
	if err != nil {
		return empty, err
	}

	artifacts := model.Artifacts{Dir: dir, Files: files}

	err = VerifyArtifacts(artifacts)
	if err != nil {
		return empty, err
	}

	return artifacts, nil
}

func openArtifactOutput(dir string) (*os.Root, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("inspect artifact output: %w", err)
	}

	if !info.IsDir() {
		return nil, errArtifactOutputNotDirectory
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open artifact output: %w", err)
	}

	return root, nil
}

func artifactManifestInventory(data []byte) ([]model.ArtifactFile, error) {
	var manifest otelc.BundleManifest

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	err := decoder.Decode(&manifest)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact manifest: %w", err)
	}

	err = decoder.Decode(new(any))
	if !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errUnsupportedArtifactManifest, err)
		}

		return nil, errUnsupportedArtifactManifest
	}

	if manifest.APIVersion != "otelplan.io/artifacts/v1alpha1" || len(manifest.Files) == 0 {
		return nil, errUnsupportedArtifactManifest
	}

	files := slices.Clone(manifest.Files)
	files = append(files, model.ArtifactFile{Path: "manifest.json", Digest: artifactDigest(data)})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	return files, nil
}

// PublishArtifacts publishes a complete bundle. Clean permits replacing only a
// verified existing bundle; unexpected or modified files are never removed.
func PublishArtifacts(destination string, files []otelc.GeneratedFile, clean bool) (model.Artifacts, error) {
	var empty model.Artifacts

	destination, err := filepath.Abs(destination)
	if err != nil {
		return empty, fmt.Errorf("resolve artifact output path: %w", err)
	}

	previous, err := existingArtifactOutput(destination)
	if err != nil {
		return empty, err
	}

	parent := filepath.Dir(destination)

	err = os.MkdirAll(parent, artifactStagingDirectoryMode)
	if err != nil {
		return empty, fmt.Errorf("create artifact output parent: %w", err)
	}

	staged, err := StageArtifacts(parent, files)
	if err != nil {
		return empty, err
	}

	defer func() { _ = os.RemoveAll(staged.Dir) }()

	_, err = ReadArtifacts(staged.Dir)
	if err != nil {
		return empty, err
	}

	if previous.Dir != "" && slices.Equal(previous.Files, staged.Files) {
		return previous, nil
	}

	err = publishStagedArtifacts(destination, staged.Dir, previous.Dir != "", clean)
	if err != nil {
		return empty, err
	}

	return model.Artifacts{Dir: destination, Files: staged.Files}, nil
}

func existingArtifactOutput(destination string) (model.Artifacts, error) {
	var empty model.Artifacts

	_, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil
	}

	if err != nil {
		return empty, fmt.Errorf("inspect previous artifact output: %w", err)
	}

	return ReadArtifacts(destination)
}

func publishStagedArtifacts(destination, staged string, exists, clean bool) error {
	if exists {
		if !clean {
			return errArtifactOutputDiffers
		}

		return replaceArtifactOutput(destination, staged)
	}

	err := os.Rename(staged, destination)
	if err != nil {
		return fmt.Errorf("publish artifact output: %w", err)
	}

	return nil
}

func replaceArtifactOutput(destination, staged string) error {
	backup, err := os.MkdirTemp(filepath.Dir(destination), "otelplan-previous-")
	if err != nil {
		return fmt.Errorf("create previous artifact backup: %w", err)
	}

	old := filepath.Join(backup, "artifacts")

	err = os.Rename(destination, old)
	if err != nil {
		cleanupErr := os.Remove(backup)
		if cleanupErr != nil {
			return fmt.Errorf("move previous artifacts to backup: %w; remove empty backup: %w", err, cleanupErr)
		}

		return fmt.Errorf("move previous artifacts to backup: %w", err)
	}

	return commitArtifactReplacement(destination, staged, backup)
}

func commitArtifactReplacement(destination, staged, backup string) error {
	publishErr := os.Rename(staged, destination)
	if publishErr != nil {
		old := filepath.Join(backup, "artifacts")

		restoreErr := os.Rename(old, destination)
		if restoreErr != nil {
			return fmt.Errorf(
				"publish failed: %w; restore failed: %w; previous artifacts retained at %s",
				publishErr, restoreErr, old,
			)
		}

		cleanupErr := os.Remove(backup)
		if cleanupErr != nil {
			return fmt.Errorf("publish artifact output: %w; remove empty backup: %w", publishErr, cleanupErr)
		}

		return fmt.Errorf("publish artifact output: %w", publishErr)
	}

	err := os.RemoveAll(backup)
	if err != nil {
		return fmt.Errorf("published artifacts but could not remove previous output: %w", err)
	}

	return nil
}
