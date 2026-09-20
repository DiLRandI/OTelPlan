package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLockChecksIgnorePolicyRuleOrder(t *testing.T) {
	root, files := cliFixture(t)
	first := "- id: operation\n  match:\n    functions: [Run]\n"
	second := "- id: other\n  match:\n    functions: [Other]\n"

	header := strings.TrimSuffix(files["otelplan.yaml"], first)

	err := os.WriteFile(filepath.Join(root, "app.go"), []byte(files["app.go"]+"\nfunc Other(ctx context.Context) error { return nil }\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	policy := filepath.Join(root, "otelplan.yaml")

	err = os.WriteFile(policy, []byte(header+first+second), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	invoke(t, root, 0, "lock")

	err = os.WriteFile(policy, []byte(header+second+first), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	invoke(t, root, 0, "lock", "--check")
	invoke(t, root, 0, "diff", "--check")
}
