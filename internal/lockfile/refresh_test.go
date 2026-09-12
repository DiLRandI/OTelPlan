package lockfile

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestRefreshBuildIdentityOwnership(t *testing.T) {
	previous, current := fixtureLock(t), fixtureLock(t)
	previous.Backend.Digest = Digest([]byte("executable"))
	previous.Artifacts = []model.ArtifactFile{{Path: "rules.yaml", Digest: Digest(nil)}}
	if !ResolutionDiff(previous, current).Empty() {
		t.Fatal("resolution comparison claimed build identity drift")
	}
	if Diff(previous, current).Empty() {
		t.Fatal("full comparison lost build drift")
	}
	refreshed, err := RefreshResolution(previous, current)
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
	current.Targets[0].Location.Line++
	if _, err := RefreshResolution(previous, current); err != nil {
		t.Fatal("diagnostic coordinates invalidated build identity")
	}
	current.Targets[0].SpanName = "changed"
	if _, err := RefreshResolution(previous, current); err == nil {
		t.Fatal("old build identity retained across meaningful drift")
	}
	if _, err := RefreshResolution(fixtureLock(t), previous); err == nil {
		t.Fatal("resolution phase supplied build identity")
	}
}
