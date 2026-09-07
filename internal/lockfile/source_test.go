package lockfile

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestSourceDriftIgnoresCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*model.LockTarget)
		kind   model.DiffClassification
	}{
		{name: "line", change: func(target *model.LockTarget) { target.Location.Line += 10 }},
		{name: "column", change: func(target *model.LockTarget) { target.Location.Column += 3 }},
		{name: "file", change: func(target *model.LockTarget) { target.Location.File = "moved.go" }, kind: model.DiffSource},
		{name: "signature", change: func(target *model.LockTarget) {
			target.Signature = "func(int)"
			target.SignatureDigest = Digest([]byte(target.Signature))
		}, kind: model.DiffSignature},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, after := fixtureLock(t), fixtureLock(t)
			tc.change(&after.Targets[0])
			diff := Diff(before, after)
			if tc.kind == "" {
				if !diff.Empty() {
					t.Fatalf("coordinates caused drift: %+v", diff)
				}
				return
			}
			if len(diff.Entries) != 1 || diff.Entries[0].Classification != tc.kind {
				t.Fatalf("meaningful drift missing: %+v", diff)
			}
		})
	}
}
