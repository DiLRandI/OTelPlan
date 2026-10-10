package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/internal/policy"
	"github.com/DiLRandI/OTelPlan/internal/resolve"
	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const APIVersion = "otelplan.io/cli/v1alpha1"

const (
	initCommandName = "init"
	jsonFormat      = "json"
	exitUsage       = 2
	commandUsage    = "usage: otelplan [global flags] " +
		"<init|scan|inspect|explain|validate|lock|diff|compile|build|version> [arguments]"
)

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

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return run(ctx, args, nil, stdout, stderr)
}

// RunWithInput runs the CLI with explicit input for interactive commands.
func RunWithInput(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return run(ctx, args, stdin, stdout, stderr)
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts, positionals, err := parse(args)
	if err != nil {
		command := ""
		if len(positionals) > 0 {
			command = positionals[0]
		}

		return usageError(opts, command, err.Error(), stdout, stderr)
	}

	if opts.verbose {
		opts.details = new(diagnosticDetails)
	}

	if opts.help {
		positionals = []string{"help"}
	}

	if len(positionals) == 0 {
		return usageError(opts, "", commandUsage, stdout, stderr)
	}

	command := positionals[0]
	rest := positionals[1:]
	output := response{APIVersion: APIVersion, Command: command, OK: true, Diagnostics: model.DiagnosticErrorList{}}

	fail := func(exit int, code model.Code, message string) int {
		output.OK = false

		output.Diagnostics = append(output.Diagnostics, model.DiagnosticError{Severity: model.SeverityError, Code: code, Message: message})

		err := emit(stdout, opts, output)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)

			return 1
		}

		return exit
	}
	if opts.check && opts.dryRun {
		return fail(2, model.CodeInvalidPolicy, "cannot combine --check and --dry-run")
	}

	if (opts.dependencies || opts.interfaces || opts.callGraph) && command != "scan" {
		return fail(2, model.CodeInvalidPolicy, "--dependencies, --interfaces, and --calls are supported by scan")
	}

	policyCommand := command == "inspect" || command == "explain" || command == "validate" || command == "lock" || command == "diff" || command == "compile" || command == "build"
	if (opts.strict || opts.allowLargePlan || opts.configSet) && !policyCommand {
		return fail(2, model.CodeInvalidPolicy, "--config, --strict, and --allow-large-plan require a policy command")
	}

	if command == "compile" && opts.output == "" {
		return fail(2, model.CodeInvalidPolicy, "--output must not be empty")
	}

	if opts.outputSet && command != "compile" && command != initCommandName {
		return fail(exitUsage, model.CodeInvalidPolicy, "--output is supported by init and compile")
	}

	if opts.clean && command != "compile" {
		return fail(exitUsage, model.CodeInvalidPolicy, "--clean is supported by compile")
	}

	if (opts.force || opts.interactive || opts.nonInteractive) && command != initCommandName {
		return fail(exitUsage, model.CodeInvalidPolicy, "--force, --interactive, and --non-interactive are supported by init")
	}

	if opts.interactive && opts.nonInteractive {
		return fail(exitUsage, model.CodeInvalidPolicy, "cannot combine --interactive and --non-interactive")
	}

	if opts.interactive && opts.format == jsonFormat {
		return fail(exitUsage, model.CodeInvalidPolicy, "--interactive requires text output")
	}

	if opts.interactive && stdin == nil {
		return fail(exitUsage, model.CodeInvalidPolicy, "--interactive requires an input stream")
	}

	if opts.check && command != "lock" && command != "diff" {
		return fail(2, model.CodeInvalidPolicy, "--check is supported by lock and diff")
	}

	if opts.dryRun && command != "lock" {
		return fail(2, model.CodeInvalidPolicy, "--dry-run is supported by lock")
	}

	exitCode := 0

	switch command {
	case "help":
		if len(rest) > 0 {
			return fail(2, model.CodeInvalidPolicy, "help takes no positional arguments")
		}

		output.Data = commandUsage + "\ninit [packages...] writes a starter policy; " +
			"scan [packages...] lists Go symbols; inspect resolves policy; " +
			"explain <symbol> shows rule decisions"
	case "version":
		if len(rest) > 0 {
			return fail(2, model.CodeInvalidPolicy, "version takes no positional arguments")
		}

		output.Data = map[string]string{"otelplan": Version, "go": runtime.Version()}
	case "scan":
		inventory, err := discovery.LoadContext(ctx, discovery.Options{
			Root: opts.root, Patterns: rest, IncludeDependencies: opts.dependencies,
			CallGraph: opts.callGraph, Offline: opts.offline,
		})
		if err != nil {
			recordFailure(opts, model.CodeUnresolvedSymbol, "analyze Go project", err)

			return fail(4, model.CodeUnresolvedSymbol, err.Error())
		}

		recordBuildContext(opts, inventory.EffectiveBuild)
		output.Data = inventory
	case initCommandName:
		var diagnostics model.DiagnosticErrorList

		output.Data, exitCode, diagnostics = initCommand(ctx, opts, rest, stdin, stderr)
		output.Diagnostics = append(output.Diagnostics, diagnostics...)
		output.OK = exitCode == 0
	case "inspect", "explain", "validate", "lock", "diff", "compile", "build":
		if command == "explain" && len(rest) != 1 {
			return fail(2, model.CodeInvalidPolicy, "explain requires one canonical symbol")
		}

		if command != "explain" && command != "build" && len(rest) != 0 {
			return fail(2, model.CodeInvalidPolicy, command+" takes no positional arguments")
		}

		var buildArgs buildArguments

		if command == "build" {
			var err error

			buildArgs, err = parseBuildArguments(rest)
			if err != nil {
				return fail(2, model.CodeInvalidPolicy, err.Error())
			}
		}

		config := opts.config
		if !filepath.IsAbs(config) {
			config = filepath.Join(opts.root, config)
		}

		p, err := policy.Load(config)
		if err != nil {
			recordFailure(opts, model.CodeInvalidPolicy, "load policy", err)

			return fail(3, model.CodeInvalidPolicy, "cannot read or parse policy at "+config)
		}

		output.Diagnostics = policy.Validate(p)
		if output.Diagnostics.HasErrors() {
			output.OK = false

			err := emit(stdout, opts, output)
			if err != nil {
				_, _ = fmt.Fprintln(stderr, err)

				return 1
			}

			return 3
		}

		inventory, err := discovery.LoadContext(ctx, discovery.Options{Root: opts.root, Patterns: p.Project.Packages, BuildFlags: buildArgs.AnalysisFlags, BuildTags: p.Project.BuildTags, IncludeTests: p.Project.IncludeTests, IncludeDependencies: p.Project.IncludeDependencies, Offline: opts.offline})
		if err != nil {
			recordFailure(opts, model.CodeUnresolvedSymbol, "analyze policy packages", err)

			return fail(4, model.CodeUnresolvedSymbol, err.Error())
		}

		recordBuildContext(opts, inventory.EffectiveBuild)
		result := resolve.Resolve(p, inventory)

		output.Diagnostics = append(result.Diagnostics, validate.Safety(inventory, result.Plan, validate.Options{AllowLargePlan: opts.allowLargePlan})...)
		if output.Diagnostics == nil {
			output.Diagnostics = model.DiagnosticErrorList{}
		}

		backendDiags := otelc.Check(p.Backend.Version, inventory, result.Plan)
		output.Diagnostics = append(output.Diagnostics, backendDiags...)

		output.OK = !output.Diagnostics.HasErrors() && (!opts.strict || len(output.Diagnostics.Warnings()) == 0)
		if !output.OK {
			exitCode = 5
		}

		if backendDiags.HasErrors() {
			exitCode = 7
		}

		if command == "inspect" {
			output.Data = previewPlan(result.Plan)
		} else if command == "explain" {
			for _, explanation := range result.Explanations {
				if string(explanation.SymbolID) == rest[0] {
					output.Data = explanation

					break
				}
			}

			if output.Data == nil {
				return fail(5, model.CodeUnresolvedSymbol, "symbol does not exist or policy could not be resolved")
			}
		} else if output.OK {
			var diags model.DiagnosticErrorList

			switch command {
			case "build":
				output.Data, exitCode, diags = buildCommand(ctx, opts, buildArgs, p, inventory, result.Plan)
			case "compile":
				output.Data, exitCode, diags = compileCommand(ctx, opts, p, inventory, result.Plan)
			default:
				output.Data, exitCode, diags = lockCommand(command, opts, p, inventory, result.Plan)
			}

			output.Diagnostics = append(output.Diagnostics, diags...)
			output.OK = exitCode == 0
		}
	default:
		return fail(2, model.CodeInvalidPolicy, "unknown command: "+command)
	}

	if err := emit(stdout, opts, output); err != nil {
		_, _ = fmt.Fprintln(stderr, err)

		return 1
	}

	return exitCode
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
