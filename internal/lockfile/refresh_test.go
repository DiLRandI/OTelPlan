package lockfile_test

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestRefreshBuildIdentityOwnership(t *testing.T) {
	t.Parallel()

	previous, current := fixtureLock(t), fixtureLock(t)
	previous.Backend.Digest = lockfile.Digest([]byte("executable"))

	previous.Artifacts = []model.ArtifactFile{{Path: "rules.yaml", Digest: lockfile.Digest(nil)}}

	if !lockfile.ResolutionDiff(previous, current).Empty() {
		t.Fatal("resolution comparison claimed build identity drift")
	}

	if lockfile.Diff(previous, current).Empty() {
		t.Fatal("full comparison lost build drift")
	}

	refreshed, err := lockfile.RefreshResolution(previous, current)
	if err != nil {
		t.Fatal(err)
	}

	if refreshed.Backend.Digest != previous.Backend.Digest || len(refreshed.Artifacts) != 1 {
		t.Fatal("build identity not preserved")
	}

	refreshed.Artifacts[0].Path = "changed.yaml"

	if previous.Artifacts[0].Path != "rules.yaml" || current.Backend.Digest != "" || len(current.Artifacts) != 0 {
		t.Fatal("refresh mutated caller state")
	}
}

func TestRefreshBuildIdentityDrift(t *testing.T) {
	t.Parallel()

	previous, current := fixtureLock(t), fixtureLock(t)
	previous.Backend.Digest = lockfile.Digest([]byte("executable"))
	previous.Artifacts = []model.ArtifactFile{{Path: "rules.yaml", Digest: lockfile.Digest(nil)}}

	current.Targets[0].Location.Line++

	_, err := lockfile.RefreshResolution(previous, current)
	if err != nil {
		t.Fatalf("diagnostic coordinates invalidated build identity: %v", err)
	}

	current.Targets[0].SpanName = "changed"

	_, err = lockfile.RefreshResolution(previous, current)
	if err == nil {
		t.Fatal("old build identity retained across meaningful drift")
	}

	_, err = lockfile.RefreshResolution(fixtureLock(t), previous)
	if err == nil {
		t.Fatal("resolution phase supplied build identity")
	}
}
