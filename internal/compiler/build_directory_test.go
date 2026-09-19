package compiler

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestPreparedBuildDirectory(t *testing.T) {
	source, parent := t.TempDir(), t.TempDir()
	copied := filepath.Join(parent, "module")
	nested := filepath.Join(parent, "nested")
	for _, dir := range []string{filepath.Join(copied, "cmd", "app"), filepath.Join(nested, "cmd")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	workspace := PreparedWorkspace{Dir: parent, Relocations: map[string]string{source: copied, filepath.Join(source, "nested"): nested}}
	for _, tc := range []struct{ original, want string }{{source, copied}, {filepath.Join(source, "cmd", "app"), filepath.Join(copied, "cmd", "app")}, {filepath.Join(source, "nested", "cmd"), filepath.Join(nested, "cmd")}} {
		got, err := workspace.BuildDirectory(tc.original)
		if err != nil || got != tc.want {
			t.Fatalf("build directory = %s, %v; want %s", got, err, tc.want)
		}
	}
	for _, path := range []string{"relative", source + "-other", filepath.Join(source, "missing")} {
		if _, err := workspace.BuildDirectory(path); err == nil {
			t.Fatal("accepted unknown working directory")
		}
	}
	for _, path := range []string{source, parent, t.TempDir()} {
		if err := workspace.validateBuildDirectory(path); err == nil {
			t.Fatal("accepted directory outside copied modules")
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(copied, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := workspace.BuildDirectory(filepath.Join(source, "escape")); err == nil {
		t.Fatal("accepted symlink escape")
	}
}

func TestApplicationBuildDirectory(t *testing.T) {
	parent, source := t.TempDir(), t.TempDir()
	copied, runtime := filepath.Join(parent, "app"), filepath.Join(parent, "runtime")
	for _, dir := range []string{copied, runtime} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	workspace := PreparedWorkspace{Dir: parent, WorkspaceFile: filepath.Join(parent, "go.work"), Relocations: map[string]string{source: copied}}
	workspace.Runtime.Dir = runtime
	for _, tc := range []struct{ name, uses, want string }{
		{"skip runtime", "use (\n" + strconv.Quote(runtime) + "\n" + strconv.Quote(copied) + "\n)\n", copied},
		{"relative member", "use ./app\n", copied},
		{"runtime only", "use ./runtime\n", ""},
		{"outside copy", "use " + strconv.Quote(source) + "\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(workspace.WorkspaceFile, []byte("go 1.27.0\n"+tc.uses), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := workspace.applicationBuildDirectory()
			if tc.want == "" {
				if err == nil {
					t.Fatal("accepted invalid application directory")
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("directory=%q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
