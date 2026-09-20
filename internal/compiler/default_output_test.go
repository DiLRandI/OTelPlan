package compiler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultBuildOutput(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "library"), 0o700); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{"go.mod": "module example.com/tool/v2\n\ngo 1.27.0\n", "main.go": "package main\nfunc main(){}\n", "library/lib.go": "package library\n"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ target, goos, want string }{{".", "linux", "tool"}, {".", "windows", "tool.exe"}, {"main.go", "linux", "main"}, {"./library", "linux", ""}, {"./...", "linux", ""}} {
		got, err := defaultBuildOutput(t.Context(), root, append(os.Environ(), "GOWORK=off", "GOFLAGS="), nil, []string{tc.target}, tc.goos)
		if err != nil || got != tc.want {
			t.Fatalf("%s output=%s, %v; want %s", tc.target, got, err, tc.want)
		}
	}
}
