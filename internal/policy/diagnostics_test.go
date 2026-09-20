package policy_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/policy"
)

func TestValidateDiagnosticOrder(t *testing.T) {
	t.Parallel()

	parsed, err := policy.Parse([]byte(invalidDiagnosticPolicy))
	if err != nil {
		t.Fatal(err)
	}

	actual, err := json.Marshal(policy.Validate(parsed))
	if err != nil {
		t.Fatal(err)
	}

	var expected bytes.Buffer

	err = json.Compact(&expected, []byte(expectedPolicyDiagnostics))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(actual, expected.Bytes()) {
		t.Fatalf("diagnostics = %s, want %s", actual, expected.Bytes())
	}
}

const invalidDiagnosticPolicy = `
apiVersion: bad
kind: Other
backend: {name: unknown}
defaults:
  spanName: "{{unknown}}"
  context: {mode: bad}
  attributes: {arguments: true}
rules:
  - id: dup
    match: {functions: [""], ownership: foreign}
    exclude: {}
    span: {kind: server, name: "{{bad}}"}
    attributes:
      - {key: kind, from: {constant: x}}
      - {key: kind, from: {constant: y}}
  - id: dup
    match: {functions: [Run]}
exclusions:
  - id: dup
    match: {}
`

const expectedPolicyDiagnostics = `[
  {"severity":"error","code":"OTP4005",
   "message":"blanket argument/result capture is prohibited; use explicit attribute rules"},
  {"severity":"error","code":"OTP1005","message":"unsupported default context mode"},
  {"severity":"error","code":"OTP1004","message":"defaults.spanName: unknown template variable \"unknown\""},
  {"severity":"error","code":"OTP1005","message":"unsupported backend"},
  {"severity":"error","code":"OTP1005",
   "message":"apiVersion \"bad\" is not supported, want \"otelplan.io/v1alpha1\""},
  {"severity":"error","code":"OTP1005","message":"kind \"Other\" is not supported, want \"InstrumentationPlan\""},
  {"severity":"error","code":"OTP5002",
   "message":"backend.version must be pinned to an exact version for lock/build workflows"},
  {"severity":"error","code":"OTP1001","message":"rule \"dup\": exclude must select at least one field"},
  {"severity":"error","code":"OTP1005",
   "message":"rule \"dup\": span.kind \"server\" is not supported, only \"internal\""},
  {"severity":"error","code":"OTP1004","message":"rule \"dup\": span.name: unknown template variable \"bad\""},
  {"severity":"error","code":"OTP1005","message":"duplicate attribute key","rule":"dup"},
  {"severity":"error","code":"OTP1001","message":"unknown ownership selector","rule":"dup"},
  {"severity":"error","code":"OTP1001","message":"selector values must not be empty","rule":"dup"},
  {"severity":"error","code":"OTP1003","message":"duplicate rule id \"dup\""},
  {"severity":"error","code":"OTP1005",
   "message":"exclusion ID must be valid and unique across rules and exclusions","rule":"dup"},
  {"severity":"error","code":"OTP1001","message":"exclusion \"dup\": match must select at least one field"}
]`

func TestValidateMissingPolicyDiagnostic(t *testing.T) {
	t.Parallel()

	actual, err := json.Marshal(policy.Validate(nil))
	if err != nil {
		t.Fatal(err)
	}

	const expected = `[{"severity":"error","code":"OTP1005","message":"policy is required"}]`
	if string(actual) != expected {
		t.Fatalf("diagnostics = %s, want %s", actual, expected)
	}
}
