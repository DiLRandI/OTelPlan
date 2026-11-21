package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
)

func TestPackageAnalysisDiagnosticWithholdsCompilerText(t *testing.T) {
	t.Parallel()

	const fixture = `package app
const Invalid int = "local-source-text-marker"
`

	for _, name := range []string{"scan", "inspect", "init"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root, _, _ := policyExecutionFixture(t, fixture, executionPolicy)

			for _, format := range []string{"text", "json"} {
				checkRedactedAnalysisFailure(t, name, root, format)
			}
		})
	}
}

func checkRedactedAnalysisFailure(t *testing.T, name, root, format string) {
	t.Helper()

	var stdout, stderr bytes.Buffer

	args := []string{name, "--root", root, "--offline", "--verbose", "--format=" + format}
	if name == "init" {
		args = append(args, "--output=starter.yaml")
	}

	code := cli.Run(t.Context(), args, &stdout, &stderr)
	report := stdout.String() + stderr.String()

	if code != 4 || !strings.Contains(report, "Go package analysis failed") || !strings.Contains(report, "OTP1002") {
		t.Fatalf("expected analysis failure in %s: exit=%d output=%q", format, code, report)
	}

	if strings.Contains(report, "local-source-text-marker") {
		t.Fatal("analysis diagnostic echoed source text")
	}
}
