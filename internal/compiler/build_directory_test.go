package compiler

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

type preparedDirectoryFixture struct {
	source    string
	parent    string
	copied    string
	nested    string
	workspace PreparedWorkspace
}

func newPreparedDirectoryFixture(t *testing.T) preparedDirectoryFixture {
	t.Helper()

	source, parent := t.TempDir(), t.TempDir()
	copied := filepath.Join(parent, "module")
	nested := filepath.Join(parent, "nested")

	for _, directory := range []string{
		filepath.Join(copied, "cmd", "app"),
		filepath.Join(nested, "cmd"),
	} {
		err := os.MkdirAll(directory, 0o700)
		if err != nil {
			t.Fatal(err)
		}
	}

	workspace := new(PreparedWorkspace)
	workspace.Dir = parent
	workspace.Relocations = map[string]string{
		source:                          copied,
		filepath.Join(source, "nested"): nested,
	}

	return preparedDirectoryFixture{
		source: source, parent: parent, copied: copied, nested: nested, workspace: *workspace,
	}
}

func TestPreparedBuildDirectory(t *testing.T) {
	t.Parallel()

	fixture := newPreparedDirectoryFixture(t)
	for _, testCase := range []struct{ original, want string }{
		{fixture.source, fixture.copied},
		{filepath.Join(fixture.source, "cmd", "app"), filepath.Join(fixture.copied, "cmd", "app")},
		{filepath.Join(fixture.source, "nested", "cmd"), filepath.Join(fixture.nested, "cmd")},
	} {
		got, err := fixture.workspace.BuildDirectory(testCase.original)
		if err != nil || got != testCase.want {
			t.Fatalf("build directory = %s, %v; want %s", got, err, testCase.want)
		}
	}
}

func TestPreparedBuildDirectoryRejectsUnknownWorkingDirectory(t *testing.T) {
	t.Parallel()

	fixture := newPreparedDirectoryFixture(t)
	for _, path := range []string{"relative", fixture.source + "-other", filepath.Join(fixture.source, "missing")} {
		_, err := fixture.workspace.BuildDirectory(path)
		if err == nil {
			t.Fatalf("accepted unknown working directory %q", path)
		}
	}
}

func TestPreparedBuildDirectoryRejectsUncopiedDirectories(t *testing.T) {
	t.Parallel()

	fixture := newPreparedDirectoryFixture(t)
	for _, path := range []string{fixture.source, fixture.parent, t.TempDir()} {
		err := fixture.workspace.validateBuildDirectory(path)
		if err == nil {
			t.Fatalf("accepted directory outside copied modules: %q", path)
		}
	}
}

func TestPreparedBuildDirectoryRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	fixture := newPreparedDirectoryFixture(t)
	outside := t.TempDir()

	err := os.Symlink(outside, filepath.Join(fixture.copied, "escape"))
	if err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err = fixture.workspace.BuildDirectory(filepath.Join(fixture.source, "escape"))
	if err == nil {
		t.Fatal("accepted symlink escape")
	}
}

func TestApplicationBuildDirectory(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct{ name, uses, want string }{
		{
			name: "skip runtime",
			uses: "use (\n@runtime@\n@app@\n)\n",
			want: "app",
		},
		{name: "relative member", uses: "use ./app\n", want: "app"},
		{name: "runtime only", uses: "use ./runtime\n", want: ""},
		{name: "outside copy", uses: "use @outside@\n", want: ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			testApplicationBuildDirectory(t, testCase)
		})
	}
}

func testApplicationBuildDirectory(t *testing.T, testCase struct{ name, uses, want string }) {
	t.Helper()

	parent, source := t.TempDir(), t.TempDir()
	copied := filepath.Join(parent, "app")
	runtime := filepath.Join(parent, "runtime")

	for _, directory := range []string{copied, runtime} {
		err := os.Mkdir(directory, 0o700)
		if err != nil {
			t.Fatal(err)
		}
	}

	workspace := new(PreparedWorkspace)
	workspace.Dir = parent
	workspace.WorkspaceFile = filepath.Join(parent, "go.work")
	workspace.Relocations = map[string]string{source: copied}
	workspace.Runtime.Dir = runtime

	uses := strings.ReplaceAll(testCase.uses, "@app@", strconv.Quote(copied))
	uses = strings.ReplaceAll(uses, "@runtime@", strconv.Quote(runtime))
	uses = strings.ReplaceAll(uses, "@outside@", strconv.Quote(source))

	err := os.WriteFile(workspace.WorkspaceFile, []byte("go 1.27.0\n"+uses), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	want := testCase.want
	if want != "" {
		want = copied
	}

	got, err := workspace.applicationBuildDirectory()
	if want == "" {
		if err == nil {
			t.Fatal("accepted invalid application directory")
		}

		return
	}

	if err != nil || got != want {
		t.Fatalf("directory=%q, %v; want %q", got, err, want)
	}
}

func TestPreparedWorkspaceParseFailurePreservesCause(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()

	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	err = root.WriteFile("go.work", []byte("go invalid\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	workspace := new(PreparedWorkspace)
	workspace.Dir = parent
	workspace.WorkspaceFile = filepath.Join(parent, "go.work")
	directory, err := workspace.applicationBuildDirectory()

	var parseErrors modfile.ErrorList
	if directory != "" || !errors.As(err, &parseErrors) || !strings.Contains(err.Error(), "parse prepared workspace") {
		t.Fatal("workspace parse failure lost its parser cause or operation context", err)
	}
}
