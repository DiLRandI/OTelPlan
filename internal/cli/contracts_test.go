package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCLIUsageContracts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		message string
	}{
		{"unknown flag", []string{"lock", "--unknown=private-value"}, "unknown flag"},
		{"missing value", []string{"lock", "--config"}, "requires a value"},
		{"invalid boolean", []string{"lock", "--strict=invalid"}, "invalid"},
		{"missing command", nil, "usage:"},
		{"lock positional", []string{"lock", "foo"}, "lock takes no positional arguments"},
		{"validate positional", []string{"validate", "foo"}, "validate takes no positional arguments"},
		{"conflicting flags", []string{"lock", "--check", "--dry-run"}, "cannot combine"},
		{"dependencies applicability", []string{"inspect", "--dependencies"}, "scan"},
		{"interfaces applicability", []string{"lock", "--interfaces"}, "scan"},
		{"strict applicability", []string{"scan", "--strict"}, "policy"},
		{"config applicability", []string{"scan", "--config=unused.yaml"}, "policy"},
		{"verbose unsupported", []string{"version", "--verbose"}, "not implemented"},
	} {
		for _, format := range []string{"text", "json"} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				args := append([]string{"--root", t.TempDir(), "--format=" + format}, tc.args...)
				var out, errout bytes.Buffer
				if code := Run(args, &out, &errout); code != 2 {
					t.Fatalf("usage exit=%d output=%s stderr=%s", code, &out, &errout)
				}
				if !strings.Contains(out.String()+errout.String(), tc.message) {
					t.Fatalf("wrong diagnostic: %s %s", &out, &errout)
				}
				if strings.Contains(out.String()+errout.String(), "private-value") {
					t.Fatal("argument value leaked")
				}
				if format == "json" {
					var reply response
					if err := json.Unmarshal(out.Bytes(), &reply); err != nil || reply.OK || len(reply.Diagnostics) == 0 || errout.Len() != 0 {
						t.Fatalf("invalid error envelope: %s stderr=%s", &out, &errout)
					}
				}
			})
		}
	}
}

func TestJSONFormatAfterInvalidFlag(t *testing.T) {
	for _, args := range [][]string{{"--unknown", "lock", "--format=json"}, {"lock", "--strict=invalid", "--format", "json"}} {
		var out, errout bytes.Buffer
		if code := Run(args, &out, &errout); code != 2 {
			t.Fatalf("exit=%d", code)
		}
		var reply response
		if err := json.Unmarshal(out.Bytes(), &reply); err != nil || reply.OK || len(reply.Diagnostics) == 0 || errout.Len() != 0 {
			t.Fatalf("invalid JSON error: %s %s", &out, &errout)
		}
	}
}

func TestDiffCheckFailureHasDiagnostic(t *testing.T) {
	root, _ := cliFixture(t)
	reply := invoke(t, root, 6, "diff", "--check")
	if len(reply.Diagnostics) == 0 || reply.Data == nil {
		t.Fatalf("stale diff lacks diagnostic or data: %+v", reply)
	}
}

func TestUsageJSONOutputFailure(t *testing.T) {
	var stderr bytes.Buffer
	if got := Run([]string{"--format=json", "--unknown"}, failingWriter{}, &stderr); got != 1 {
		t.Fatalf("write failure exit=%d", got)
	}
}

func TestDiffCheckTextDiagnostic(t *testing.T) {
	root, _ := cliFixture(t)
	var out, errout bytes.Buffer
	if got := Run([]string{"diff", "--check", "--root", root, "--offline"}, &out, &errout); got != 6 {
		t.Fatalf("exit=%d output=%s stderr=%s", got, &out, &errout)
	}
	if !strings.Contains(out.String(), "OTP6001") || !strings.Contains(out.String(), "lockfile") {
		t.Fatalf("missing stale diagnostic: %s", &out)
	}
}
