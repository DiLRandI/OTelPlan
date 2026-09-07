package cli

import (
	"os"
	"path/filepath"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/lockfile"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

type lockSummary struct {
	Path    string `json:"path"`
	Targets int    `json:"targets"`
	Changed bool   `json:"changed"`
	DryRun  bool   `json:"dryRun"`
}

func lockCommand(command string, opts options, p *model.Policy, code *model.CodeModel, plan model.ResolvedPlan) (any, int, model.DiagnosticList) {
	fail := func(exit int, message string) (any, int, model.DiagnosticList) {
		return nil, exit, model.DiagnosticList{{Severity: model.SeverityError, Code: model.CodeStaleLockfile, Message: message}}
	}
	graph, err := lockfile.GraphDigest(code)
	if err != nil {
		return fail(4, err.Error())
	}
	identity, err := otelc.Identity(p.Backend.Version)
	if err != nil {
		return fail(7, err.Error())
	}
	current, err := lockfile.Create(p, code, plan, identity, graph, nil)
	if err != nil {
		return fail(5, err.Error())
	}
	filename := filepath.Join(opts.root, "otelplan.lock")
	var previous model.Lockfile
	contents, err := os.ReadFile(filename)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return fail(6, "cannot read lockfile")
	}
	if exists {
		previous, err = lockfile.Parse(contents)
		if err != nil {
			if command != "lock" || opts.check {
				return fail(6, "lockfile is invalid; regenerate it with otelplan lock")
			}
			previous = model.Lockfile{}
		}
	}
	diff := lockfile.ResolutionDiff(previous, current)
	summary := lockSummary{Path: filename, Targets: len(plan.Targets), Changed: !diff.Empty(), DryRun: opts.dryRun}
	switch command {
	case "validate":
		if exists && !diff.Empty() {
			return diff, 6, model.DiagnosticList{{Severity: model.SeverityError, Code: model.CodeStaleLockfile, Message: "lockfile differs from current resolved state"}}
		}
		return summary, 0, nil
	case "diff":
		if opts.check && !diff.Empty() {
			return diff, 6, nil
		}
		return diff, 0, nil
	case "lock":
		if opts.check {
			if !exists || !diff.Empty() {
				return diff, 6, model.DiagnosticList{{Severity: model.SeverityError, Code: model.CodeStaleLockfile, Message: "lockfile is missing or stale"}}
			}
			return summary, 0, nil
		}
		current, err = lockfile.RefreshResolution(previous, current)
		if err != nil {
			return fail(6, err.Error())
		}
		if !opts.dryRun {
			if err := lockfile.Write(filename, current); err != nil {
				return fail(1, err.Error())
			}
		}
		return summary, 0, nil
	}
	return fail(2, "unsupported lockfile operation")
}
