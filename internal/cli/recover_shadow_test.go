package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/cli"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const recoverShadowSource = `package app
import "context"
func Run(context.Context) error { return nil }
var recover = func() any { panic("caller-private-value") }
`

func TestRecoverShadowingPreventsPolicyCommandWrites(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"inspect", "explain", "validate", "lock", "diff", "compile", "build"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			root, directory, files := policyExecutionFixture(t, recoverShadowSource, executionPolicy)

			args := []string{command}
			if command == "explain" {
				args = append(args, "example.com/app.Run")
			}

			reply := policyExecutionReply(t.Context(), t, root, args, 7)
			checkPolicyDiagnosticCodes(t, reply, []model.Code{model.CodeBackendUnsupported})

			diagnostic := reply.Diagnostics[0]
			if diagnostic.Symbol != "example.com/app.Run" || diagnostic.RuleID != "operation" ||
				!strings.Contains(diagnostic.Message, "shadows the recover builtin") {
				t.Fatalf("missing actionable compatibility diagnostic: %+v", diagnostic)
			}

			checkPolicyExecutionFiles(t, directory, files)
		})
	}
}

func TestRecoverShadowingTextDiagnostic(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"validate", "compile"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			root, directory, files := policyExecutionFixture(t, recoverShadowSource, executionPolicy)

			var stdout, stderr bytes.Buffer

			exit := cli.Run(t.Context(), []string{"--root", root, "--offline", command}, &stdout, &stderr)

			output := stdout.String() + stderr.String()
			if exit != 7 || !strings.Contains(output, "OTP5001") ||
				!strings.Contains(output, "shadows the recover builtin") || strings.Contains(output, "caller-private-value") {
				t.Fatalf("unexpected compatibility failure: exit=%d stdout=%s stderr=%s", exit, &stdout, &stderr)
			}

			checkPolicyExecutionFiles(t, directory, files)
		})
	}
}
