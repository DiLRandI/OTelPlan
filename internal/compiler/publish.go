package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

// ReadArtifacts verifies a bundle against its own manifest. This establishes
// file ownership for refresh, not authenticity against a trusted lockfile.
func ReadArtifacts(dir string) (model.Artifacts, error) {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return model.Artifacts{}, fmt.Errorf("artifact output must be a directory")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return model.Artifacts{}, err
	}
	defer func() { _ = root.Close() }()
	data, err := root.ReadFile("manifest.json")
	if err != nil {
		return model.Artifacts{}, fmt.Errorf("read artifact manifest")
	}
	var manifest otelc.BundleManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return model.Artifacts{}, fmt.Errorf("invalid artifact manifest")
	}
	if decoder.Decode(new(any)) != io.EOF || manifest.APIVersion != "otelplan.io/artifacts/v1alpha1" || len(manifest.Files) == 0 {
		return model.Artifacts{}, fmt.Errorf("unsupported or invalid artifact manifest")
	}
	artifacts := model.Artifacts{Dir: dir, Files: append(manifest.Files, model.ArtifactFile{Path: "manifest.json", Digest: artifactDigest(data)})}
	sort.Slice(artifacts.Files, func(i, j int) bool { return artifacts.Files[i].Path < artifacts.Files[j].Path })
	if err := VerifyArtifacts(artifacts); err != nil {
		return model.Artifacts{}, err
	}
	return artifacts, nil
}

// PublishArtifacts publishes a complete bundle. Clean permits replacing only a
// verified existing bundle; unexpected or modified files are never removed.
func PublishArtifacts(destination string, files []otelc.GeneratedFile, clean bool) (model.Artifacts, error) {
	destination, err := filepath.Abs(destination)
	if err != nil {
		return model.Artifacts{}, err
	}
	var previous model.Artifacts
	if _, err := os.Lstat(destination); err == nil {
		previous, err = ReadArtifacts(destination)
		if err != nil {
			return model.Artifacts{}, err
		}
	} else if !os.IsNotExist(err) {
		return model.Artifacts{}, err
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return model.Artifacts{}, err
	}
	staged, err := StageArtifacts(parent, files)
	if err != nil {
		return model.Artifacts{}, err
	}
	defer func() { _ = os.RemoveAll(staged.Dir) }()
	if _, err := ReadArtifacts(staged.Dir); err != nil {
		return model.Artifacts{}, err
	}
	if previous.Dir != "" {
		if slices.Equal(previous.Files, staged.Files) {
			return previous, nil
		}
		if !clean {
			return model.Artifacts{}, fmt.Errorf("artifact output differs; use --clean to replace verified output")
		}
		backup, err := os.MkdirTemp(parent, "otelplan-previous-")
		if err != nil {
			return model.Artifacts{}, err
		}
		old := filepath.Join(backup, "artifacts")
		if err := os.Rename(destination, old); err != nil {
			_ = os.Remove(backup)
			return model.Artifacts{}, err
		}
		if err := os.Rename(staged.Dir, destination); err != nil {
			if restoreErr := os.Rename(old, destination); restoreErr != nil {
				return model.Artifacts{}, fmt.Errorf("publish failed; previous artifacts retained at %s", old)
			}
			_ = os.Remove(backup)
			return model.Artifacts{}, err
		}
		if err := os.RemoveAll(backup); err != nil {
			return model.Artifacts{}, fmt.Errorf("published artifacts but could not remove previous output: %w", err)
		}
	} else if err := os.Rename(staged.Dir, destination); err != nil {
		return model.Artifacts{}, err
	}
	return model.Artifacts{Dir: destination, Files: staged.Files}, nil
}
