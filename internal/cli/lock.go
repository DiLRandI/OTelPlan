package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const (
	lockCommandName     = "lock"
	diffCommandName     = "diff"
	validateCommandName = "validate"
	exitStaleLock       = 6
)

type lockSummary struct {
	Path    string `json:"path"`
	Targets int    `json:"targets"`
	Changed bool   `json:"changed"`
	DryRun  bool   `json:"dryRun"`
}

type resolutionLockState struct {
	previous model.Lockfile
	current  model.Lockfile
	diff     model.LockDiff
	summary  lockSummary
	exists   bool
}

func lockCommand(command string, opts options, policy *model.Policy, code *model.CodeModel,
	plan model.ResolvedPlan) (any, int, model.DiagnosticErrorList) {
	state, exit, diagnostics := prepareResolutionLock(command, opts, policy, code, plan)
	if diagnostics != nil {
		return nil, exit, diagnostics
	}

	switch command {
	case validateCommandName:
		if state.exists && !state.diff.Empty() {
			return state.diff, exitStaleLock, lockDiagnostic("lockfile differs from current resolved state")
		}

		return state.summary, 0, nil
	case diffCommandName:
		if opts.check && !state.diff.Empty() {
			return state.diff, exitStaleLock, lockDiagnostic("lockfile is missing or differs from current resolved state")
		}

		return state.diff, 0, nil
	case lockCommandName:
		return refreshResolutionLock(opts, state)
	default:
		return lockFailure(exitUsage, "unsupported lockfile operation")
	}
}

func prepareResolutionLock(command string, opts options, policy *model.Policy, code *model.CodeModel,
	plan model.ResolvedPlan) (*resolutionLockState, int, model.DiagnosticErrorList) {
	graph, err := lockfile.GraphDigest(code)
	if err != nil {
		recordFailure(opts, model.CodeStaleLockfile, "fingerprint analyzed project", err)

		return nil, exitAnalysis, lockDiagnostic(err.Error())
	}

	identity, err := otelc.Identity(policy.Backend.Version)
	if err != nil {
		recordFailure(opts, model.CodeStaleLockfile, "resolve pinned backend identity", err)

		return nil, exitBackend, lockDiagnostic(err.Error())
	}

	current, err := lockfile.Create(policy, code, plan, identity, graph, nil)
	if err != nil {
		recordFailure(opts, model.CodeStaleLockfile, "construct resolution lock", err)

		return nil, exitValidation, lockDiagnostic(err.Error())
	}

	filename := filepath.Join(opts.root, "otelplan.lock")

	previous, exists, diagnostics := previousResolutionLock(command, opts, filename)
	if diagnostics != nil {
		return nil, exitStaleLock, diagnostics
	}

	state := new(resolutionLockState)
	state.previous, state.current, state.exists = previous, current, exists
	state.diff = lockfile.ResolutionDiff(previous, current)
	state.summary = lockSummary{
		Path: filename, Targets: len(plan.Targets), Changed: !state.diff.Empty(), DryRun: opts.dryRun,
	}

	return state, 0, nil
}

func previousResolutionLock(command string, opts options, filename string) (model.Lockfile, bool,
	model.DiagnosticErrorList) {
	var previous model.Lockfile

	contents, err := readResolutionLock(filename)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return previous, false, nil
		}

		recordFailure(opts, model.CodeStaleLockfile, "read resolution lock", err)

		return previous, false, lockDiagnostic("cannot read lockfile")
	}

	previous, err = lockfile.Parse(contents)
	if err == nil {
		return previous, true, nil
	}

	if command != lockCommandName || opts.check {
		recordFailure(opts, model.CodeStaleLockfile, "parse resolution lock", err)

		return previous, true, lockDiagnostic("lockfile is invalid; regenerate it with otelplan lock")
	}

	var empty model.Lockfile

	return empty, true, nil
}

func readResolutionLock(filename string) ([]byte, error) {
	resolved, err := filepath.EvalSymlinks(filename)
	if err != nil {
		return nil, fmt.Errorf("resolve lockfile: %w", err)
	}

	directory, err := os.OpenRoot(filepath.Dir(resolved))
	if err != nil {
		return nil, fmt.Errorf("open lockfile directory: %w", err)
	}

	contents, readErr := directory.ReadFile(filepath.Base(resolved))
	closeErr := directory.Close()

	if readErr != nil {
		return nil, fmt.Errorf("read lockfile: %w", readErr)
	}

	if closeErr != nil {
		return nil, fmt.Errorf("close lockfile directory: %w", closeErr)
	}

	return contents, nil
}

func refreshResolutionLock(opts options, state *resolutionLockState) (any, int, model.DiagnosticErrorList) {
	if opts.check {
		if !state.exists || !state.diff.Empty() {
			return state.diff, exitStaleLock, lockDiagnostic("lockfile is missing or stale")
		}

		return state.summary, 0, nil
	}

	current, err := lockfile.RefreshResolution(state.previous, state.current)
	if err != nil {
		recordFailure(opts, model.CodeStaleLockfile, "refresh resolution lock", err)

		return lockFailure(exitStaleLock, err.Error())
	}

	if !opts.dryRun {
		err = lockfile.Write(state.summary.Path, current)
		if err != nil {
			recordFailure(opts, model.CodeStaleLockfile, "write resolution lock", err)

			return lockFailure(1, err.Error())
		}
	}

	return state.summary, 0, nil
}

func lockFailure(exit int, message string) (any, int, model.DiagnosticErrorList) {
	return nil, exit, lockDiagnostic(message)
}

func lockDiagnostic(message string) model.DiagnosticErrorList {
	var diagnostic model.DiagnosticError

	diagnostic.Severity = model.SeverityError
	diagnostic.Code = model.CodeStaleLockfile
	diagnostic.Message = message

	return model.DiagnosticErrorList{diagnostic}
}
