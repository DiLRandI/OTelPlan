package cli

import (
	"context"
	"fmt"
	"io"
	"runtime"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

// APIVersion identifies the versioned JSON response contract.
const APIVersion = "otelplan.io/cli/v1alpha1"

const (
	helpCommandName    = "help"
	versionCommandName = "version"
	inspectCommandName = "inspect"
	explainCommandName = "explain"
	compileCommandName = "compile"
	buildCommandName   = "build"
	initCommandName    = "init"
	jsonFormat         = "json"
	exitUsage          = 2
	commandUsage       = "usage: otelplan [global flags] " +
		"<init|scan|inspect|explain|validate|lock|diff|compile|build|version> [arguments]"
)

// Version is the tool identity recorded in responses and generated artifacts.
var Version = "dev"

type response struct {
	APIVersion  string                    `json:"apiVersion"`
	Command     string                    `json:"command"`
	OK          bool                      `json:"ok"`
	Diagnostics model.DiagnosticErrorList `json:"diagnostics"`
	Data        any                       `json:"data,omitempty"`
	Details     *diagnosticDetails        `json:"details,omitempty"`
}

type options struct {
	details                                                 *diagnosticDetails
	output                                                  string
	outputSet, clean                                        bool
	strict, offline, check, dryRun, allowLargePlan, force   bool
	interactive, nonInteractive                             bool
	configSet                                               bool
	callGraph                                               bool
	root, config, format                                    string
	quiet, verbose, noColor, dependencies, interfaces, help bool
}

// Run executes arguments with caller cancellation and returns a CLI exit code.
// The caller retains ownership of the output writers.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return run(ctx, args, nil, stdout, stderr)
}

// RunWithInput supplies caller-owned input for interactive commands.
// Cancellation stops the CLI; the caller remains responsible for releasing a blocked input reader.
func RunWithInput(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return run(ctx, args, stdin, stdout, stderr)
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts, positionals, err := parse(args)
	if err != nil {
		return usageError(opts, firstCommand(positionals), err.Error(), stdout, stderr)
	}

	if opts.verbose {
		opts.details = new(diagnosticDetails)
	}

	if opts.help {
		positionals = []string{helpCommandName}
	}

	if len(positionals) == 0 {
		return usageError(opts, "", commandUsage, stdout, stderr)
	}

	command := positionals[0]
	data, exit, diagnostics := dispatchCommand(ctx, command, opts, positionals[1:], stdin, stderr)
	reply := commandResponse(command, data, exit, diagnostics)

	err = emit(stdout, opts, reply)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)

		return 1
	}

	return exit
}

func firstCommand(positionals []string) string {
	if len(positionals) == 0 {
		return ""
	}

	return positionals[0]
}

func dispatchCommand(ctx context.Context, command string, opts options, args []string,
	stdin io.Reader, stderr io.Writer) (any, int, model.DiagnosticErrorList) {
	err := validateCommandOptions(command, opts, stdin)
	if err != nil {
		return nil, exitUsage, compileDiagnostic(model.CodeInvalidPolicy, err.Error())
	}

	switch command {
	case helpCommandName, versionCommandName:
		return describeCommand(command, args)
	case "scan":
		return scanCommand(ctx, opts, args)
	case initCommandName:
		return initCommand(ctx, opts, args, stdin, stderr)
	default:
		if isPolicyCommand(command) {
			return runPolicyCommand(ctx, command, opts, args)
		}

		return nil, exitUsage, compileDiagnostic(model.CodeInvalidPolicy, "unknown command: "+command)
	}
}

func commandResponse(command string, data any, exit int, diagnostics model.DiagnosticErrorList) response {
	if diagnostics == nil {
		diagnostics = model.DiagnosticErrorList{}
	}

	return response{
		APIVersion: APIVersion, Command: command, OK: exit == 0, Diagnostics: diagnostics, Data: data, Details: nil,
	}
}

func describeCommand(command string, args []string) (any, int, model.DiagnosticErrorList) {
	if len(args) != 0 {
		return nil, exitUsage, compileDiagnostic(model.CodeInvalidPolicy, command+" takes no positional arguments")
	}

	if command == helpCommandName {
		overview := commandUsage + "\ninit [packages...] writes a starter policy; " +
			"scan [packages...] lists Go symbols; inspect resolves policy; " +
			"explain <symbol> shows rule decisions"

		return overview, 0, nil
	}

	return map[string]string{"otelplan": Version, "go": runtime.Version()}, 0, nil
}

func scanCommand(ctx context.Context, opts options, patterns []string) (any, int, model.DiagnosticErrorList) {
	var analysis discovery.Options

	analysis.Root, analysis.Patterns = opts.root, patterns
	analysis.IncludeDependencies, analysis.CallGraph, analysis.Offline = opts.dependencies, opts.callGraph, opts.offline

	inventory, err := discovery.LoadContext(ctx, analysis)
	if err != nil {
		recordFailure(opts, model.CodeUnresolvedSymbol, "analyze Go project", err)

		return nil, exitAnalysis, compileDiagnostic(model.CodeUnresolvedSymbol, err.Error())
	}

	recordBuildContext(opts, inventory.EffectiveBuild)

	return inventory, 0, nil
}

func previewPlan(plan model.ResolvedPlan) model.ResolvedPlan {
	plan.Targets = append([]model.ResolvedTarget{}, plan.Targets...)
	for i := range plan.Targets {
		plan.Targets[i].Attributes = append([]model.AttributePlan(nil), plan.Targets[i].Attributes...)
		for j := range plan.Targets[i].Attributes {
			attr := &plan.Targets[i].Attributes[j]
			if attr.From.Constant != nil {
				attr.From.Constant = "[redacted]"
			}
		}
	}

	return plan
}
