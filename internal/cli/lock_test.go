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
	if code := Run(arguments, &out, &errout); code != want {
		t.Fatalf("%v exit=%d want=%d output=%s stderr=%s", args, code, want, &out, &errout)
	}
	var reply response
	if err := json.Unmarshal(out.Bytes(), &reply); err != nil {
		t.Fatalf("invalid JSON: %s", &out)
	}
	if reply.OK != (want == 0) {
		t.Fatalf("JSON ok disagrees with exit: %+v", reply)
	}
	return reply
}

func TestLockCommandsAndDrift(t *testing.T) {
	root, files := cliFixture(t)
	filename := filepath.Join(root, "otelplan.lock")
	invoke(t, root, 0, "validate", "--strict")
	invoke(t, root, 6, "lock", "--check")
	invoke(t, root, 0, "lock", "--dry-run")
	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		t.Fatal("dry-run wrote lockfile")
	}
	invoke(t, root, 0, "lock")
	original, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	invoke(t, root, 0, "lock", "--check")
	invoke(t, root, 0, "diff", "--check")
	changed := strings.Replace(files["app.go"], "ctx context.Context", "ctx context.Context, value int", 1)
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte(changed), 0644); err != nil {
		t.Fatal(err)
	}
	invoke(t, root, 6, "lock", "--check")
	invoke(t, root, 0, "diff")
	invoke(t, root, 6, "diff", "--check")
	invoke(t, root, 6, "validate")
	after, err := os.ReadFile(filename)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("check/diff rewrote lockfile")
	}
	invoke(t, root, 0, "lock")
	invoke(t, root, 0, "validate", "--strict")
	if err := os.WriteFile(filename, []byte("invalid lockfile"), 0644); err != nil {
		t.Fatal(err)
	}
	invoke(t, root, 6, "lock", "--check")
	invoke(t, root, 0, "lock")
	invoke(t, root, 0, "lock", "--check")
	got, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil || string(got) != files["go.mod"] {
		t.Fatal("lock commands changed module")
	}
}

func TestStrictWarningsAndUnsupportedBackend(t *testing.T) {
	root, files := cliFixture(t)
	contents := files["otelplan.yaml"] + "  attributes:\n  - key: user_id\n    from: {constant: stable-test-id}\n"
	filename := filepath.Join(root, "otelplan.yaml")
	if err := os.WriteFile(filename, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	invoke(t, root, 0, "validate")
	invoke(t, root, 5, "validate", "--strict")
	if err := os.WriteFile(filename, []byte(strings.Replace(files["otelplan.yaml"], "v1.1.0", "v9.9.9", 1)), 0644); err != nil {
		t.Fatal(err)
	}
	invoke(t, root, 7, "validate")
}
