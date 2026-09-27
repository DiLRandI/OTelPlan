package lockfile_test

import (
	"path/filepath"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
)

func TestGraphVersionedReplacementIgnoresLocalDirectories(t *testing.T) {
	t.Parallel()

	code := graphFixture(t)
	replacement := code.Modules[1].Replace
	replacement.Path = "example.com/replacement"
	replacement.Version = "v1.2.3"
	replacement.Dir = filepath.Join(t.TempDir(), "absent-replacement")
	code.Modules[1].Dir = filepath.Join(t.TempDir(), "absent-original")

	before, err := lockfile.GraphDigest(code)
	if err != nil {
		t.Fatalf("fingerprint versioned replacement without local metadata: %v", err)
	}

	replacement.Dir = filepath.Join(t.TempDir(), "another-absent-replacement")

	relocated, err := lockfile.GraphDigest(code)
	if err != nil || relocated != before {
		t.Fatalf("unused local directory changed fingerprint: %v", err)
	}

	replacement.Version = "v1.2.4"

	changed, err := lockfile.GraphDigest(code)
	if err != nil || changed == before {
		t.Fatalf("replacement version change was not fingerprinted: %v", err)
	}
}
