package lockfile_test

import (
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestValidationErrorsDescribeRejectedState(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		change  func(*model.Lockfile)
		message string
	}{
		{
			name: "context index", message: "invalid context index",
			change: func(lock *model.Lockfile) { lock.Targets[0].Context.Index = -1 },
		},
		{
			name: "error index", message: "invalid error index",
			change: func(lock *model.Lockfile) { lock.Targets[0].Errors.Indexes = []int{-1} },
		},
		{
			name: "attribute key", message: "invalid attribute key",
			change: func(lock *model.Lockfile) { lock.Targets[0].Attributes[0].Key = "" },
		},
		{
			name: "attribute kind", message: "invalid attribute kind",
			change: func(lock *model.Lockfile) { lock.Targets[0].Attributes[0].Kind = "unsupported" },
		},
		{
			name: "backend digest", message: "invalid backend digest",
			change: func(lock *model.Lockfile) { lock.Backend.Digest = "not-a-digest" },
		},
		{
			name: "artifact path", message: "invalid artifact manifest",
			change: func(lock *model.Lockfile) {
				lock.Artifacts = []model.ArtifactFile{{Path: "../rules.yaml", Digest: lockfile.Digest(nil)}}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			lock := fixtureLock(t)
			lock.Targets[0].Attributes[0].From.Constant = "private-value-must-not-appear"
			testCase.change(&lock)

			_, err := lockfile.Marshal(lock)
			if err == nil || err.Error() != testCase.message {
				t.Fatalf("Marshal error = %v; want %q", err, testCase.message)
			}

			if strings.Contains(err.Error(), "private-value-must-not-appear") {
				t.Fatal("validation error exposed an attribute value")
			}
		})
	}
}
