package lockfile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/lockfile"
)

func TestGraphRejectsInvalidWorkspaceModule(t *testing.T) {
	t.Parallel()

	code := graphFixture(t)
	moduleDir := filepath.Join(t.TempDir(), "invalid")

	err := os.Mkdir(moduleDir, 0o700)
	if err != nil {
		t.Fatalf("create workspace fixture directory: %v", err)
	}

	err = os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte("go 1.27\n"), 0o600)
	if err != nil {
		t.Fatalf("write workspace fixture manifest: %v", err)
	}

	workspace := "go 1.27\n\nuse " + filepath.ToSlash(moduleDir) + "\n"

	err = os.WriteFile(code.WorkspaceFile, []byte(workspace), 0o600)
	if err != nil {
		t.Fatalf("write workspace fixture: %v", err)
	}

	digest, err := lockfile.GraphDigest(code)
	if err == nil || !strings.Contains(err.Error(), "workspace module has invalid manifest") {
		t.Fatalf("GraphDigest error = %v; want invalid workspace module", err)
	}

	if digest != "" {
		t.Fatal("failed workspace fingerprint returned a digest")
	}
}
