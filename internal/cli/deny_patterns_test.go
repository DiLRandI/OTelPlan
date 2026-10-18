package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const denyPatternSentinel = "deny-pattern-private-sentinel"

func TestCLIDenyPatternsRejectSensitiveAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		patterns     string
		attributeKey string
	}{
		{name: "custom pattern", patterns: "[bank_reference]", attributeKey: "bank_reference"},
		{name: "case and separator normalization", patterns: "[BANK_REFERENCE]", attributeKey: "bank.reference"},
		{name: "built in with omitted list", patterns: "", attributeKey: "password"},
		{name: "built in with empty list", patterns: "[]", attributeKey: "password"},
		{name: "built in with unrelated list", patterns: "[customer_id]", attributeKey: "password"},
		{name: "public acknowledgment does not bypass", patterns: "[bank_reference]", attributeKey: "bank_reference"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			safety := ""
			if testCase.name == "public acknowledgment does not bypass" {
				safety = "\n    safety: {classification: public, allow: true}"
			}

			root, directory, files := policyExecutionFixture(t,
				"package app\nimport \"context\"\nfunc Run(context.Context) error { return nil }\n",
				denyPatternPolicy(testCase.patterns, testCase.attributeKey, safety))

			for _, format := range []string{"text", "json"} {
				output, reply, exit := runDenyPatternInspect(t, root, format)
				if exit != 5 {
					t.Fatalf("format=%s exit=%d, want 5; output=%s", format, exit, output)
				}

				if strings.Contains(output, denyPatternSentinel) {
					t.Fatalf("format=%s exposed captured constant: %s", format, output)
				}

				if format == "json" {
					if reply.OK || len(reply.Diagnostics) != 1 || reply.Diagnostics[0].Code != model.CodeSecretAttribute {
						t.Fatalf("unexpected JSON response: %+v", reply)
					}
				}
			}

			checkPolicyExecutionFiles(t, directory, files)
		})
	}
}

func TestCLIDenyPatternSecretAcknowledgment(t *testing.T) {
	t.Parallel()

	root, directory, files := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(context.Context) error { return nil }\n",
		denyPatternPolicy("[bank_reference]", "bank.reference",
			"\n    safety: {classification: secret, allow: true}"))

	reply := runGlobalArgumentsJSON(t, []string{
		"--root", root, "--offline", "--format=json", "inspect",
	}, 0)
	if !reply.OK || len(reply.Diagnostics) != 0 {
		t.Fatalf("secret acknowledgment rejected: %+v", reply)
	}

	checkPolicyExecutionFiles(t, directory, files)
}

func TestCLIDenyPatternChecksArgumentPath(t *testing.T) {
	t.Parallel()

	source := "package app\nimport \"context\"\n" +
		"type Request struct { BankReference string }\n" +
		"func Run(context.Context, Request) error { return nil }\n"
	configured := denyPatternPolicy("[bank_reference]", "operation.kind", "")
	configured = strings.Replace(configured, "from: {constant: "+denyPatternSentinel+"}",
		"from: {argument: 1.BankReference}", 1)
	root, directory, files := policyExecutionFixture(t, source, configured)

	reply := runGlobalArgumentsJSON(t, []string{
		"--root", root, "--offline", "--format=json", "inspect",
	}, 5)
	if reply.OK || len(reply.Diagnostics) != 1 || reply.Diagnostics[0].Code != model.CodeSecretAttribute {
		t.Fatalf("custom sensitive argument path was accepted: %+v", reply)
	}

	checkPolicyExecutionFiles(t, directory, files)
}

func TestCLIDenyPatternInvalidPolicyExit(t *testing.T) {
	t.Parallel()

	root, directory, files := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(context.Context) error { return nil }\n",
		denyPatternPolicy("[bank reference]", "bank_reference", ""))

	reply := runGlobalArgumentsJSON(t, []string{
		"--root", root, "--offline", "--format=json", "inspect",
	}, 3)
	if reply.OK || len(reply.Diagnostics) != 1 || reply.Diagnostics[0].Code != model.CodeInvalidPolicy ||
		strings.Contains(reply.Diagnostics[0].Message, "bank reference") {
		t.Fatalf("unexpected invalid-policy response: %+v", reply)
	}

	checkPolicyExecutionFiles(t, directory, files)
}

func TestCLIDenyPatternsRefreshSafetyWithAnalysisCache(t *testing.T) {
	t.Parallel()

	root, directory, files := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(context.Context) error { return nil }\n",
		denyPatternPolicy("", "bank_reference", ""))
	cacheDir := filepath.Join(t.TempDir(), "analysis-cache")
	args := []string{"--root", root, "--offline", "--cache-dir", cacheDir, "--format=json", "inspect"}

	first := runGlobalArgumentsJSON(t, args, 0)
	if !first.OK || len(first.Diagnostics) != 0 {
		t.Fatalf("initial cached inspection failed: %+v", first)
	}

	updated := denyPatternPolicy("[bank_reference]", "bank_reference", "")

	err := directory.WriteFile("otelplan.yaml", []byte(updated), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	files["otelplan.yaml"] = updated

	second := runGlobalArgumentsJSON(t, args, 5)
	if second.OK || len(second.Diagnostics) != 1 || second.Diagnostics[0].Code != model.CodeSecretAttribute {
		t.Fatalf("cached safety settings bypassed: %+v", second)
	}

	checkPolicyExecutionFiles(t, directory, files)
}

func TestCLIDenyPatternsLockIdentity(t *testing.T) {
	t.Parallel()

	root, directory, files := policyExecutionFixture(t,
		"package app\nimport \"context\"\nfunc Run(context.Context) error { return nil }\n",
		denyPatternPolicy("[bank_reference, account_number]", "operation.kind", ""))
	runResolutionCommand(t, root, 0, "lock")

	locked, err := directory.ReadFile("otelplan.lock")
	if err != nil {
		t.Fatal(err)
	}

	files["otelplan.lock"] = string(locked)
	files["otelplan.yaml"] = denyPatternPolicy("[account_number, bank_reference, bank_reference]", "operation.kind", "")

	err = directory.WriteFile("otelplan.yaml", []byte(files["otelplan.yaml"]), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	runResolutionCommand(t, root, 0, "lock", "--check")
	checkPolicyExecutionFiles(t, directory, files)
	files["otelplan.yaml"] = denyPatternPolicy("[credit_card]", "operation.kind", "")

	err = directory.WriteFile("otelplan.yaml", []byte(files["otelplan.yaml"]), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	runResolutionCommand(t, root, 6, "lock", "--check")
	runResolutionCommand(t, root, 6, "diff", "--check")
	checkPolicyExecutionFiles(t, directory, files)
}

func denyPatternPolicy(patterns, attributeKey, safety string) string {
	policy := executionPolicy
	if patterns != "" {
		policy = strings.Replace(policy, "rules:", "denyPatterns: "+patterns+"\nrules:", 1)
	}

	return policy + fmt.Sprintf("  attributes:\n  - key: %s\n    from: {constant: %s}%s\n",
		attributeKey, denyPatternSentinel, safety)
}

func runDenyPatternInspect(t *testing.T, root, format string) (string, globalArgumentReply, int) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	exit := cli.Run(t.Context(), []string{
		"--root", root, "--offline", "--format=" + format, "inspect",
	}, &stdout, &stderr)
	if stderr.Len() != 0 {
		t.Fatalf("format=%s stderr=%s", format, stderr.String())
	}

	var reply globalArgumentReply

	if format == "json" {
		err := json.Unmarshal(stdout.Bytes(), &reply)
		if err != nil {
			t.Fatalf("decode JSON response: %v; output=%s", err, stdout.String())
		}
	}

	return stdout.String(), reply, exit
}
