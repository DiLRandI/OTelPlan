package discovery_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
)

func TestLoadReportsWorkspaceChecksumReadFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for name, contents := range map[string]string{
		"go.work": "go 1.27\n\nuse .\n",
		"go.mod":  "module example.com/workspace\n\ngo 1.27\n",
		"app.go":  "package workspace\nfunc Run() {}\n",
	} {
		err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600)
		if err != nil {
			t.Fatalf("write workspace fixture %s: %v", name, err)
		}
	}

	err := os.Mkdir(filepath.Join(root, "go.work.sum"), 0o700)
	if err != nil {
		t.Fatalf("create unreadable checksum fixture: %v", err)
	}

	_, err = discovery.Load(discovery.Options{
		Root: root, Patterns: nil, BuildTags: nil, BuildFlags: nil, CallGraph: false,
		IncludeTests: false, IncludeDependencies: false, GOOS: "", GOARCH: "", Offline: true,
		Env: []string{"GOWORK=" + filepath.Join(root, "go.work"), "GOFLAGS="},
	})
	if err == nil || !strings.Contains(err.Error(), "read workspace checksums:") {
		t.Fatalf("Load error = %v; want workspace checksum read context", err)
	}

	_, filesystemError := errors.AsType[*os.PathError](err)
	if !filesystemError {
		t.Fatalf("Load error does not retain filesystem cause: %v", err)
	}
}
