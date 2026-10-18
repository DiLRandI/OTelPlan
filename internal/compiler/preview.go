package compiler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

var errArtifactOutputParentNotDirectory = errors.New("artifact output parent must be a directory")

// PreviewArtifacts checks publication of a staged bundle without changing the
// destination. The staged inventory must come from trusted generation.
func PreviewArtifacts(destination string, staged model.Artifacts, clean bool) (model.Artifacts, error) {
	var empty model.Artifacts

	destination, err := filepath.Abs(destination)
	if err != nil {
		return empty, fmt.Errorf("resolve artifact output path: %w", err)
	}

	previous, err := existingArtifactOutput(destination)
	if err != nil {
		return empty, err
	}

	err = checkArtifactOutputParent(destination)
	if err != nil {
		return empty, err
	}

	verified, err := ReadArtifacts(staged.Dir)
	if err != nil {
		return empty, err
	}

	expected := slices.Clone(staged.Files)
	sort.Slice(expected, func(i, j int) bool { return expected[i].Path < expected[j].Path })

	if !slices.Equal(verified.Files, expected) {
		return empty, errStagedArtifactDigestMismatch
	}

	if previous.Dir != "" && !slices.Equal(previous.Files, verified.Files) && !clean {
		return empty, errArtifactOutputDiffers
	}

	return model.Artifacts{Dir: destination, Files: verified.Files}, nil
}

func checkArtifactOutputParent(destination string) error {
	parent := filepath.Dir(destination)

	for {
		info, err := os.Stat(parent)
		if err == nil {
			if !info.IsDir() {
				return errArtifactOutputParentNotDirectory
			}

			return nil
		}

		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect artifact output parent: %w", err)
		}

		_, linkErr := os.Lstat(parent)
		if linkErr == nil {
			return fmt.Errorf("inspect artifact output parent: %w", err)
		}

		if !errors.Is(linkErr, os.ErrNotExist) {
			return fmt.Errorf("inspect artifact output parent: %w", linkErr)
		}

		next := filepath.Dir(parent)
		if next == parent {
			return fmt.Errorf("inspect artifact output parent: %w", err)
		}

		parent = next
	}
}
