package lockfile_test

import (
	"reflect"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestDiffOrdersMetadataAndTargetChanges(t *testing.T) {
	t.Parallel()

	previous, current := fixtureLock(t), fixtureLock(t)
	removed := previous.Targets[0]
	removed.Symbol = "example.com/app.Old"
	previous.Targets = append(previous.Targets, removed)

	added := current.Targets[0]
	added.Symbol = "example.com/app.New"
	current.Targets = append(current.Targets, added)
	current.GoVersion = "go1.27.1"
	current.PolicyDigest = lockfile.Digest([]byte("changed policy"))
	current.Targets[0].SourceRule = "new-rule"
	current.Targets[0].SpanName = "new-span"

	want := model.LockDiff{Entries: []model.LockDiffEntry{
		{Classification: model.DiffBuild, Symbol: "", Detail: "Go toolchain or module graph changed"},
		{Classification: model.DiffPolicy, Symbol: "", Detail: "policy digest changed"},
		{Classification: model.DiffAdd, Symbol: added.Symbol, Detail: "target added"},
		{Classification: model.DiffRemove, Symbol: removed.Symbol, Detail: "target removed"},
		{Classification: model.DiffPolicy, Symbol: current.Targets[0].Symbol, Detail: "source rule changed"},
		{Classification: model.DiffSpanName, Symbol: current.Targets[0].Symbol, Detail: "span name changed"},
	}}
	if got := lockfile.Diff(previous, current); !reflect.DeepEqual(got, want) {
		t.Fatalf("Diff = %+v; want %+v", got, want)
	}

	if current.Targets[0].Symbol != "example.com/app.Run" || previous.Targets[0].Symbol != "example.com/app.Run" {
		t.Fatal("Diff reordered caller-owned targets")
	}
}
