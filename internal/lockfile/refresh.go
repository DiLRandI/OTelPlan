package lockfile

import (
	"fmt"
	"slices"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

// ResolutionDiff compares analysis-owned state without asserting that an
// executable or generated artifacts have been verified.
func ResolutionDiff(previous, current model.Lockfile) model.LockDiff {
	previous.Backend.Digest, current.Backend.Digest = "", ""
	previous.Artifacts, current.Artifacts = nil, nil
	return Diff(previous, current)
}

// RefreshResolution preserves build-owned identity only while its associated
// resolution is unchanged. A build must replace that identity after drift.
func RefreshResolution(previous, current model.Lockfile) (model.Lockfile, error) {
	if current.Backend.Digest != "" || len(current.Artifacts) > 0 {
		return model.Lockfile{}, fmt.Errorf("resolution refresh cannot supply build-owned identity")
	}
	if previous.Backend.Digest == "" && len(previous.Artifacts) == 0 {
		return current, nil
	}
	if !ResolutionDiff(previous, current).Empty() {
		return model.Lockfile{}, fmt.Errorf("resolution changed while the lock contains build-owned identity; refresh requires regenerated build metadata")
	}
	current.Backend.Digest = previous.Backend.Digest
	current.Artifacts = slices.Clone(previous.Artifacts)
	return current, nil
}
