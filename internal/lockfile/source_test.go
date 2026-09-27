package lockfile_test

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestSourceDriftIgnoresCoordinates(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		change func(*model.LockTarget)
		kind   model.DiffClassification
	}{
		{name: "line", change: func(target *model.LockTarget) { target.Location.Line += 10 }, kind: ""},
		{name: "column", change: func(target *model.LockTarget) { target.Location.Column += 3 }, kind: ""},
		{name: "file", change: func(target *model.LockTarget) { target.Location.File = "moved.go" }, kind: model.DiffSource},
		{name: "signature", change: func(target *model.LockTarget) {
			target.Signature = "func(int)"
			target.SignatureDigest = lockfile.Digest([]byte(target.Signature))
		}, kind: model.DiffSignature},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			before, after := fixtureLock(t), fixtureLock(t)
			testCase.change(&after.Targets[0])

			diff := lockfile.Diff(before, after)
			if testCase.kind == "" {
				if !diff.Empty() {
					t.Fatalf("coordinates caused drift: %+v", diff)
				}

				return
			}

			if len(diff.Entries) != 1 || diff.Entries[0].Classification != testCase.kind {
				t.Fatalf("meaningful drift missing: %+v", diff)
			}
		})
	}
}
