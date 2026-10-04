package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultBuildOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	err := os.Mkdir(filepath.Join(root, "library"), 0o700)
	if err != nil {
		t.Fatal(err)
	}

	for path, data := range map[string]string{
		"go.mod":         "module example.com/tool/v2\n\ngo 1.27.0\n",
		"main.go":        "package main\nfunc main(){}\n",
		"library/lib.go": "package library\n",
	} {
		err := os.WriteFile(filepath.Join(root, path), []byte(data), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	env := append(os.Environ(), "GOWORK=off", "GOFLAGS=")

	for _, targetCase := range []struct{ target, goos, want string }{
		{target: ".", goos: "linux", want: "tool"},
		{target: ".", goos: "windows", want: "tool.exe"},
		{target: "main.go", goos: "linux", want: "main"},
		{target: "./library", goos: "linux", want: ""},
		{target: "./...", goos: "linux", want: ""},
	} {
		t.Run(targetCase.target+"/"+targetCase.goos, func(t *testing.T) {
			t.Parallel()

			got, err := defaultBuildOutput(t.Context(), root, env, nil, []string{targetCase.target}, targetCase.goos, "readonly")
			if err != nil || got != targetCase.want {
				t.Fatalf("%s output=%s, %v; want %s", targetCase.target, got, err, targetCase.want)
			}
		})
	}
}

func TestBuildTargetQueryRejectsCompilerOverrides(t *testing.T) {
	t.Parallel()

	const sensitive = "private-value-must-not-be-printed"
	for _, flag := range []string{"-toolexec", "-overlay", "-gcflags", "-ldflags", "-C"} {
		_, err := defaultBuildOutput(t.Context(), t.TempDir(), nil,
			[]string{flag + "=" + sensitive}, []string{"."}, "linux", "readonly")
		if err == nil || !strings.Contains(err.Error(), "unsupported target query flag") ||
			strings.Contains(err.Error(), sensitive) {
			t.Fatal("compiler override was not rejected safely before package loading", err)
		}
	}
}

func TestBuildTargetQueryFlags(t *testing.T) {
	t.Parallel()

	for _, flags := range [][]string{
		nil, {"-tags=a,b"}, {"-tags", "a,b"}, {"--tags="},
		{"-race", "-trimpath=false", "-buildvcs=auto"}, {"-msan=false", "-asan=false"},
	} {
		err := validateTargetQueryFlags(flags, "readonly")
		if err != nil {
			t.Fatal("valid analyzed query settings rejected", err)
		}
	}

	for _, flags := range [][]string{{"-tags"}, {"race=true"}, {"-race=invalid"}, {"-buildvcs=invalid"}} {
		err := validateTargetQueryFlags(flags, "readonly")
		if err == nil {
			t.Fatal("invalid query settings accepted")
		}
	}
}

func TestBuildTargetQueryOperandsCannotOverrideMode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	directory, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = directory.Close() }()

	err = directory.WriteFile("go.mod", []byte("module example.com/tool\n\ngo 1.27\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = directory.WriteFile("main.go", []byte("package main\nfunc main() {}\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(), "GOWORK=off", "GOFLAGS=")

	name, err := defaultBuildOutput(t.Context(), root, env, nil, []string{"-mod=mod"}, "linux", "readonly")
	if err == nil || name != "" {
		t.Fatal("package operand silently replaced the analyzed module mode")
	}
}

func TestDecodeDefaultOutputMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, data, want string
		invalid          bool
	}{
		{name: "versioned module", data: `{ "Name":"main", "ImportPath":"example.com/tool/v2" }`,
			want: "tool", invalid: false},
		{name: "cgo command", data: `{ "Name":"main", "ImportPath":"command-line-arguments", "CgoFiles":["native.go"] }`,
			want: "native", invalid: false},
		{name: "Go file first", data: `{ "Name":"main", "ImportPath":"command-line-arguments",` +
			` "GoFiles":["first.go"], "CgoFiles":["native.go"] }`, want: "first", invalid: false},
		{name: "no sources", data: `{ "Name":"main", "ImportPath":"command-line-arguments" }`, want: "", invalid: true},
		{name: "malformed", data: `{broken`, want: "", invalid: true},
		{name: "empty", data: "", want: "", invalid: false},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			name, err := decodeDefaultBuildOutput([]byte(testCase.data), "linux")
			if (err != nil) != testCase.invalid || name != testCase.want {
				t.Fatalf("name=%s error=%v; want name=%s invalid=%v", name, err, testCase.want, testCase.invalid)
			}
		})
	}
}
