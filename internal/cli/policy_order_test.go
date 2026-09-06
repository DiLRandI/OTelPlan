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
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte(files["app.go"]+"\nfunc Other(ctx context.Context) error { return nil }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	policy := filepath.Join(root, "otelplan.yaml")
	if err := os.WriteFile(policy, []byte(header+first+second), 0600); err != nil {
		t.Fatal(err)
	}
	invoke(t, root, 0, "lock")
	if err := os.WriteFile(policy, []byte(header+second+first), 0600); err != nil {
		t.Fatal(err)
	}
	invoke(t, root, 0, "lock", "--check")
	invoke(t, root, 0, "diff", "--check")
}
