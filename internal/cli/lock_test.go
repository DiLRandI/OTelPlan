package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func invoke(t *testing.T, root string, want int, args ...string) response {
	t.Helper()

	var out, errout bytes.Buffer

	arguments := append([]string{"--root", root, "--format=json", "--offline"}, args...)
	if code := Run(t.Context(), arguments, &out, &errout); code != want {
		t.Fatalf("%v exit=%d want=%d output=%s stderr=%s", args, code, want, &out, &errout)
	}

	var reply response

	err := json.Unmarshal(out.Bytes(), &reply)
	if err != nil {
		t.Fatalf("invalid JSON: %s", &out)
	}

	if reply.OK != (want == 0) {
		t.Fatalf("JSON ok disagrees with exit: %+v", reply)
	}

	return reply
}

func TestLockCommandsAndDrift(t *testing.T) {
	t.Parallel()

	root, files := cliFixture(t)
	fsRoot := openLockTestRoot(t, root)
	invoke(t, root, 0, "validate", "--strict")
	invoke(t, root, 6, "lock", "--check")
	invoke(t, root, 0, "lock", "--dry-run")
	assertMissing(t, fsRoot, "otelplan.lock")

	invoke(t, root, 0, "lock")
	original := readLockTestFile(t, fsRoot, "otelplan.lock")
	invoke(t, root, 0, "lock", "--check")
	invoke(t, root, 0, "diff", "--check")

	changed := strings.Replace(files["app.go"], "ctx context.Context", "ctx context.Context, value int", 1)
	writeLockTestFile(t, fsRoot, "app.go", changed)
	invoke(t, root, 6, "lock", "--check")
	invoke(t, root, 0, "diff")
	invoke(t, root, 6, "diff", "--check")
	invoke(t, root, 6, "validate")
	assertEqualFile(t, fsRoot, "otelplan.lock", string(original))
	assertEqualFile(t, fsRoot, "app.go", changed)

	invoke(t, root, 0, "lock")
	invoke(t, root, 0, "validate", "--strict")
	assertEqualFile(t, fsRoot, "app.go", changed)

	writeLockTestFile(t, fsRoot, "otelplan.lock", "invalid lockfile")
	malformed := readLockTestFile(t, fsRoot, "otelplan.lock")
	assertDiagnostic(t, invoke(t, root, 6, "lock", "--check"), "lockfile is invalid")
	assertDiagnostic(t, invoke(t, root, 6, "diff"), "lockfile is invalid")
	assertDiagnostic(t, invoke(t, root, 6, "validate"), "lockfile is invalid")
	assertEqualFile(t, fsRoot, "otelplan.lock", string(malformed))

	invoke(t, root, 0, "lock", "--dry-run")
	assertEqualFile(t, fsRoot, "otelplan.lock", string(malformed))
	invoke(t, root, 0, "lock")
	invoke(t, root, 0, "lock", "--check")
	assertEqualFile(t, fsRoot, "app.go", changed)
	assertEqualFile(t, fsRoot, "go.mod", files["go.mod"])
}

func TestLockUnreadablePathIsReportedWithoutMutation(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"lock", "diff", "validate"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			root, _ := cliFixture(t)
			fsRoot := openLockTestRoot(t, root)
			requireTestSuccess(t, fsRoot.Mkdir("otelplan.lock", 0o700), "create lockfile directory")

			writeLockTestFile(t, fsRoot, "otelplan.lock/keep", "preserve")

			args := []string{command}
			if command == "lock" {
				args = append(args, "--dry-run")
			}

			reply := invoke(t, root, 6, args...)
			assertDiagnostic(t, reply, "cannot read lockfile")
			assertEqualFile(t, fsRoot, "otelplan.lock/keep", "preserve")

			info, err := fsRoot.Stat("otelplan.lock")
			if err != nil || !info.IsDir() {
				t.Fatalf("lockfile directory changed: info=%v error=%v", info, err)
			}
		})
	}
}

func TestLockCommandsReadValidSymlinkWithoutChangingIt(t *testing.T) {
	t.Parallel()

	root, _ := cliFixture(t)
	fsRoot := openLockTestRoot(t, root)
	invoke(t, root, 0, "lock")

	err := fsRoot.Mkdir("locks", 0o700)
	if err != nil {
		t.Fatalf("create target directory: %v", err)
	}

	err = fsRoot.Rename("otelplan.lock", "locks/otelplan.lock")
	if err != nil {
		t.Fatalf("move lockfile into target directory: %v", err)
	}

	err = fsRoot.Symlink("locks/otelplan.lock", "otelplan.lock")
	if err != nil {
		t.Fatalf("link lockfile: %v", err)
	}

	targetBefore := readLockTestFile(t, fsRoot, "locks/otelplan.lock")

	invoke(t, root, 0, "lock", "--check")
	invoke(t, root, 0, "diff")
	invoke(t, root, 0, "validate")
	assertLockSymlink(t, fsRoot)
	assertEqualFile(t, fsRoot, "locks/otelplan.lock", string(targetBefore))

	invoke(t, root, 0, "lock")

	info, err := fsRoot.Lstat("otelplan.lock")
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("lock refresh did not replace symlink with a regular file: info=%v error=%v", info, err)
	}

	assertEqualFile(t, fsRoot, "locks/otelplan.lock", string(targetBefore))
	invoke(t, root, 0, "lock", "--check")
}

func TestLockReadCommandsPreserveExternalSymlinkTarget(t *testing.T) {
	t.Parallel()

	root, _ := cliFixture(t)
	fsRoot := openLockTestRoot(t, root)
	invoke(t, root, 0, "lock")

	targetRoot := openLockTestRoot(t, t.TempDir())
	target := readLockTestFile(t, fsRoot, "otelplan.lock")
	writeLockTestFile(t, targetRoot, "otelplan.lock", string(target))
	requireTestSuccess(t, fsRoot.Remove("otelplan.lock"), "remove original lockfile")

	targetPath := filepath.Join(targetRoot.Name(), "otelplan.lock")
	linkPath := filepath.Join(root, "otelplan.lock")
	requireTestSuccess(t, os.Symlink(targetPath, linkPath), "create external lockfile symlink")

	invoke(t, root, 0, "lock", "--check")
	invoke(t, root, 0, "diff")
	invoke(t, root, 0, "validate")

	link, err := fsRoot.Readlink("otelplan.lock")
	if err != nil || link != targetPath {
		t.Fatalf("external lockfile symlink changed: target=%q error=%v", link, err)
	}

	assertEqualFile(t, targetRoot, "otelplan.lock", string(target))
}

func assertLockSymlink(t *testing.T, root *os.Root) {
	t.Helper()

	link, err := root.Readlink("otelplan.lock")
	if err != nil || link != "locks/otelplan.lock" {
		t.Fatalf("lockfile symlink changed: target=%q error=%v", link, err)
	}
}

func TestStrictWarningsAndUnsupportedBackend(t *testing.T) {
	t.Parallel()

	root, files := cliFixture(t)
	fsRoot := openLockTestRoot(t, root)
	contents := files["otelplan.yaml"] + "  attributes:\n  - key: user_id\n    from: {constant: stable-test-id}\n"
	writeLockTestFile(t, fsRoot, "otelplan.yaml", contents)
	invoke(t, root, 0, "validate")
	invoke(t, root, 5, "validate", "--strict")

	unsupported := strings.Replace(files["otelplan.yaml"], "v1.1.0", "v9.9.9", 1)
	writeLockTestFile(t, fsRoot, "otelplan.yaml", unsupported)
	invoke(t, root, 7, "validate")
}

func openLockTestRoot(t *testing.T, name string) *os.Root {
	t.Helper()

	root, err := os.OpenRoot(name)
	if err != nil {
		t.Fatalf("open test root: %v", err)
	}

	t.Cleanup(func() {
		err := root.Close()
		if err != nil {
			t.Errorf("close test root: %v", err)
		}
	})

	return root
}

func readLockTestFile(t *testing.T, root *os.Root, name string) []byte {
	t.Helper()

	contents, err := root.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	return contents
}

func writeLockTestFile(t *testing.T, root *os.Root, name, contents string) {
	t.Helper()

	if parent := filepath.Dir(name); parent != "." {
		err := root.MkdirAll(parent, 0o700)
		if err != nil {
			t.Fatalf("create parent for %s: %v", name, err)
		}
	}

	err := root.WriteFile(name, []byte(contents), 0o600)
	if err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func assertMissing(t *testing.T, root *os.Root, name string) {
	t.Helper()

	_, err := root.Stat(name)
	if !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent, stat error=%v", name, err)
	}
}

func assertEqualFile(t *testing.T, root *os.Root, name, want string) {
	t.Helper()

	got := readLockTestFile(t, root, name)
	if string(got) != want {
		t.Fatalf("%s changed: got %q, want %q", name, got, want)
	}
}

func assertDiagnostic(t *testing.T, reply response, message string) {
	t.Helper()

	for _, diagnostic := range reply.Diagnostics {
		if string(diagnostic.Code) == "OTP6001" && strings.Contains(diagnostic.Message, message) {
			return
		}
	}

	t.Fatalf("missing OTP6001 diagnostic containing %q: %+v", message, reply.Diagnostics)
}

func requireTestSuccess(t *testing.T, err error, operation string) {
	t.Helper()

	if err != nil {
		t.Fatalf("%s: %v", operation, err)
	}
}
