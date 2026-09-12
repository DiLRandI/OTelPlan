package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestResolutionCommandsPreserveBuildIdentity(t *testing.T) {
	root, files := cliFixture(t)
	invoke(t, root, 0, "lock")
	filename := filepath.Join(root, "otelplan.lock")
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	locked, err := lockfile.Parse(contents)
	if err != nil {
		t.Fatal(err)
	}
	locked.Backend.Digest = lockfile.Digest([]byte("verified executable"))
	locked.Artifacts = []model.ArtifactFile{{Path: "rules.yaml", Digest: lockfile.Digest([]byte("generated rules"))}}
	if err := lockfile.Write(filename, locked); err != nil {
		t.Fatal(err)
	}
	invoke(t, root, 0, "lock", "--check")
	invoke(t, root, 0, "diff", "--check")
	invoke(t, root, 0, "validate")
	invoke(t, root, 0, "lock")
	preserved, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	after, err := lockfile.Parse(preserved)
	if err != nil {
		t.Fatal(err)
	}
	if after.Backend.Digest != locked.Backend.Digest || len(after.Artifacts) != 1 || after.Artifacts[0] != locked.Artifacts[0] {
		t.Fatal("lock refresh erased build identity")
	}
	changed := strings.Replace(files["app.go"], "ctx context.Context", "ctx context.Context, value int", 1)
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	invoke(t, root, 6, "lock", "--check")
	invoke(t, root, 6, "diff", "--check")
	invoke(t, root, 6, "lock")
	invoke(t, root, 6, "lock", "--dry-run")
	unchanged, err := os.ReadFile(filename)
	if err != nil || !bytes.Equal(unchanged, preserved) {
		t.Fatal("resolution change overwrote independently owned build metadata")
	}
}
