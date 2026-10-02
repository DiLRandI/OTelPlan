package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"strings"
	"syscall"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const maximumDiagnosticCauses = 16

type diagnosticDetails struct {
	Build    *diagnosticBuildContext `json:"build,omitempty"`
	Failures []diagnosticFailure     `json:"failures,omitempty"`
}

type diagnosticBuildContext struct {
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	ModuleMode string `json:"moduleMode"`
	Workspace  bool   `json:"workspace"`
}

type diagnosticFailure struct {
	Code   model.Code `json:"code"`
	Stage  string     `json:"stage"`
	Causes []string   `json:"causes"`
}

func recordFailure(opts options, code model.Code, stage string, cause error) {
	if opts.details == nil || cause == nil {
		return
	}

	opts.details.Failures = append(opts.details.Failures, diagnosticFailure{
		Code: code, Stage: stage, Causes: safeDiagnosticCauses(cause),
	})
}

func recordBuildContext(opts options, build model.BuildEnvironment) {
	if opts.details == nil {
		return
	}

	opts.details.Build = &diagnosticBuildContext{
		GOOS: build.GOOS, GOARCH: build.GOARCH, ModuleMode: build.ModuleMode, Workspace: build.Workspace,
	}
}

func safeDiagnosticCauses(cause error) []string {
	var causes []string

	pending := []error{cause}
	for len(pending) > 0 && len(causes) < maximumDiagnosticCauses {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]

		if current == nil {
			continue
		}

		causes = append(causes, safeCauseText(current))
		if joined, ok := current.(interface{ Unwrap() []error }); ok {
			children := joined.Unwrap()
			for index := min(len(children), maximumDiagnosticCauses) - 1; index >= 0; index-- {
				pending = append(pending, children[index])
			}

			continue
		}

		wrapped := errors.Unwrap(current)
		if wrapped != nil {
			pending = append(pending, wrapped)
		}
	}

	if len(pending) > 0 {
		causes[len(causes)-1] = "additional causes omitted"
	}

	return causes
}

func safeCauseText(cause error) string {
	if _, joined := cause.(interface{ Unwrap() []error }); joined {
		return "multiple underlying causes"
	}

	if _, wrapped := cause.(interface{ Unwrap() error }); wrapped {
		return "operation failed"
	}

	// Inspect terminal errors only so error classification cannot traverse an unbounded wrapper cycle.
	if exitError, ok := errors.AsType[*exec.ExitError](cause); ok {
		return fmt.Sprintf("subprocess exited with status %d", exitError.ExitCode())
	}

	if errno, ok := errors.AsType[syscall.Errno](cause); ok {
		return errno.Error()
	}

	return safeTerminalCauseText(cause)
}

func safeTerminalCauseText(cause error) string {
	switch {
	case errors.Is(cause, context.Canceled):
		return "operation canceled"
	case errors.Is(cause, context.DeadlineExceeded):
		return "operation deadline exceeded"
	case errors.Is(cause, fs.ErrNotExist):
		return "file or executable does not exist"
	case errors.Is(cause, fs.ErrPermission):
		return "permission denied"
	case errors.Is(cause, exec.ErrNotFound):
		return "executable not found on PATH"
	default:
		return "error details redacted"
	}
}

func emitDiagnosticDetails(out io.Writer, opts options) error {
	details := opts.details
	if details == nil {
		return nil
	}

	var text strings.Builder

	if build := details.Build; build != nil && !opts.quiet {
		fmt.Fprintf(&text, "BUILD goos=%s goarch=%s module-mode=%s workspace=%t\n",
			build.GOOS, build.GOARCH, build.ModuleMode, build.Workspace)
	}

	for _, failure := range details.Failures {
		fmt.Fprintf(&text, "DETAIL %s stage=%s\n", failure.Code, failure.Stage)

		for _, cause := range failure.Causes {
			fmt.Fprintf(&text, "  cause: %s\n", cause)
		}
	}

	_, err := io.WriteString(out, text.String())
	if err != nil {
		return fmt.Errorf("write verbose diagnostic details: %w", err)
	}

	return nil
}
