package resolve_test

import (
	"errors"
	"path"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/resolve"
)

func TestGlobValidatesUnreachableSegments(t *testing.T) {
	t.Parallel()

	for _, pattern := range []string{"missing/[", "**/[", "missing/\\"} {
		t.Run(pattern, func(t *testing.T) {
			t.Parallel()

			matched, err := resolve.Glob(pattern, "different")
			if matched || !errors.Is(err, path.ErrBadPattern) {
				t.Fatalf("Glob(%q) = %v, %v; want wrapped invalid-pattern error", pattern, matched, err)
			}

			if !strings.Contains(err.Error(), "invalid glob") {
				t.Fatalf("error lacks operation context: %v", err)
			}
		})
	}
}
