package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/internal/policy"
	"github.com/DiLRandI/OTelPlan/internal/resolve"
	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const APIVersion = "otelplan.io/cli/v1alpha1"

var Version = "dev"

type response struct {
	APIVersion  string               `json:"apiVersion"`
	Command     string               `json:"command"`
	OK          bool                 `json:"ok"`
	Diagnostics model.DiagnosticList `json:"diagnostics"`
	Data        any                  `json:"data,omitempty"`
}

type options struct {
	strict, offline, check, dryRun, allowLargePlan          bool
	root, config, format                                    string
	quiet, verbose, noColor, dependencies, interfaces, help bool
}

func Run(args []string, stdout, stderr io.Writer) int {
	opts, positionals, err := parse(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if opts.help {
		positionals = []string{"help"}
	}
	if len(positionals) == 0 {
		fmt.Fprintln(stderr, "usage: otelplan [global flags] <scan|inspect|explain|validate|lock|diff|version> [arguments]")
		return 2
	}
	command := positionals[0]
	rest := positionals[1:]
	output := response{APIVersion: APIVersion, Command: command, OK: true, Diagnostics: model.DiagnosticList{}}
	fail := func(exit int, code model.Code, message string) int {
		output.OK = false
		output.Diagnostics = append(output.Diagnostics, model.Diagnostic{Severity: model.SeverityError, Code: code, Message: message})
		if err := emit(stdout, opts, output); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return exit
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
		output.Data = "usage: otelplan [--root path] [--config path] [--format text|json] <scan|inspect|explain|validate|lock|diff|version>\nscan [packages...] lists Go symbols; inspect resolves policy; explain <symbol> shows rule decisions"
	case "version":
		if len(rest) > 0 {
			return fail(2, model.CodeInvalidPolicy, "version takes no positional arguments")
		}
		output.Data = map[string]string{"otelplan": Version, "go": runtime.Version()}
	case "scan":
		inventory, err := discovery.Load(discovery.Options{Root: opts.root, Patterns: rest, IncludeDependencies: opts.dependencies, Offline: opts.offline})
		if err != nil {
			return fail(4, model.CodeUnresolvedSymbol, err.Error())
		}
		output.Data = inventory
	case "inspect", "explain", "validate", "lock", "diff":
		if (command != "explain" && len(rest) != 0) || (command == "explain" && len(rest) != 1) {
			return fail(2, model.CodeInvalidPolicy, "inspect takes no arguments; explain requires one canonical symbol")
		}
		config := opts.config
		if !filepath.IsAbs(config) {
			config = filepath.Join(opts.root, config)
		}
		p, err := policy.Load(config)
		if err != nil {
			return fail(3, model.CodeInvalidPolicy, "cannot read or parse policy at "+config)
		}
		output.Diagnostics = policy.Validate(p)
		if output.Diagnostics.HasErrors() {
			output.OK = false
			if err := emit(stdout, opts, output); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			return 3
		}
		inventory, err := discovery.Load(discovery.Options{Root: opts.root, Patterns: p.Project.Packages, BuildTags: p.Project.BuildTags, IncludeTests: p.Project.IncludeTests, IncludeDependencies: p.Project.IncludeDependencies, Offline: opts.offline})
		if err != nil {
			return fail(4, model.CodeUnresolvedSymbol, err.Error())
		}
		result := resolve.Resolve(p, inventory)
		output.Diagnostics = append(result.Diagnostics, validate.Safety(inventory, result.Plan, validate.Options{AllowLargePlan: opts.allowLargePlan})...)
		if output.Diagnostics == nil {
			output.Diagnostics = model.DiagnosticList{}
		}
		backendDiags := otelc.Check(p.Backend.Version, inventory, result.Plan)
		output.Diagnostics = append(output.Diagnostics, backendDiags...)
		output.OK = !output.Diagnostics.HasErrors() && !(opts.strict && len(output.Diagnostics.Warnings()) > 0)
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
			var diags model.DiagnosticList
			output.Data, exitCode, diags = lockCommand(command, opts, p, inventory, result.Plan)
			output.Diagnostics = append(output.Diagnostics, diags...)
			output.OK = exitCode == 0
		}
	default:
		return fail(2, model.CodeInvalidPolicy, "unknown command: "+command)
	}
	if err := emit(stdout, opts, output); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return exitCode
}

func parse(args []string) (options, []string, error) {
	var opts options
	flags := flag.NewFlagSet("otelplan", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.root, "root", ".", "project root")
	flags.StringVar(&opts.config, "config", "otelplan.yaml", "policy path relative to root")
	flags.StringVar(&opts.format, "format", "text", "text or json")
	flags.BoolVar(&opts.strict, "strict", false, "fail on warnings")
	flags.BoolVar(&opts.offline, "offline", false, "disable Go network resolution")
	flags.BoolVar(&opts.check, "check", false, "check without writing")
	flags.BoolVar(&opts.dryRun, "dry-run", false, "preview without writing")
	flags.BoolVar(&opts.allowLargePlan, "allow-large-plan", false, "acknowledge large target count")
	flags.BoolVar(&opts.help, "help", false, "show usage")
	flags.BoolVar(&opts.help, "h", false, "show usage")
	flags.BoolVar(&opts.quiet, "quiet", false, "suppress informational text")
	flags.BoolVar(&opts.verbose, "verbose", false, "include selection details")
	flags.BoolVar(&opts.noColor, "no-color", false, "disable color")
	flags.BoolVar(&opts.dependencies, "dependencies", false, "include dependency code in scan")
	flags.BoolVar(&opts.interfaces, "interfaces", false, "show interface methods in text scans")
	var flagArgs, positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		name, _, hasValue := strings.Cut(name, "=")
		option := flags.Lookup(name)
		if option == nil {
			return opts, nil, fmt.Errorf("unknown flag: %s", arg)
		}
		flagArgs = append(flagArgs, arg)
		boolean, ok := option.Value.(interface{ IsBoolFlag() bool })
		if !hasValue && !(ok && boolean.IsBoolFlag()) {
			i++
			if i == len(args) {
				return opts, nil, fmt.Errorf("flag --%s requires a value", name)
			}
			flagArgs = append(flagArgs, args[i])
		}
	}
	if err := flags.Parse(flagArgs); err != nil {
		return opts, nil, err
	}
	if opts.format != "text" && opts.format != "json" {
		return opts, nil, fmt.Errorf("format must be text or json")
	}
	return opts, positionals, nil
}

func emit(out io.Writer, opts options, reply response) error {
	if opts.format == "json" {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(reply)
	}
	for _, diag := range reply.Diagnostics {
		if _, err := fmt.Fprintln(out, diag.Error()); err != nil {
			return err
		}
	}
	if opts.quiet {
		return nil
	}
	var text strings.Builder
	switch data := reply.Data.(type) {
	case string:
		fmt.Fprintln(&text, data)
	case *model.CodeModel:
		for _, symbol := range data.Symbols {
			fmt.Fprintf(&text, "%s\n  signature %s\n  source %s:%d\n  context %v  errors %v\n", symbol.ID, symbol.Signature, symbol.Location.File, symbol.Location.Line, symbol.ContextIndexes, symbol.ErrorIndexes)
		}
		if opts.interfaces {
			for _, binding := range data.InterfaceMethods {
				fmt.Fprintf(&text, "IMPLEMENTS %s %s\n", binding.InterfaceID, binding.SymbolID)
			}
		}
	case model.ResolvedPlan:
		for _, target := range data.Targets {
			fmt.Fprintf(&text, "SELECTED %s\n  span %s\n  context %s[%d]\n  errors record=%t indexes=%v\n  rule %s\n", target.SymbolID, target.SpanName, target.ContextStrategy.Strategy, target.ContextStrategy.Index, target.ErrorStrategy.Record, target.ErrorStrategy.Indexes, target.RuleID)
			for _, attr := range target.Attributes {
				source := "constant"
				if attr.From.Argument != "" {
					source = "argument " + attr.From.Argument
				}
				if attr.From.Result != "" {
					source = "result " + attr.From.Result
				}
				fmt.Fprintf(&text, "  attribute %s from %s\n", attr.Key, source)
			}
		}
		for _, skip := range data.Skipped {
			fmt.Fprintf(&text, "SKIPPED %s\n  rule %s: %s\n", skip.SymbolID, skip.RuleID, skip.Reason)
		}
	case resolve.Explanation:
		fmt.Fprintf(&text, "%s selected=%t\n", data.SymbolID, data.Selected)
		for _, decision := range data.Decisions {
			fmt.Fprintf(&text, "  %s %s: %s\n", decision.RuleID, decision.Stage, decision.Reason)
		}
	case lockSummary:
		fmt.Fprintf(&text, "%s targets=%d changed=%t dry-run=%t\n", data.Path, data.Targets, data.Changed, data.DryRun)
	case model.LockDiff:
		for _, entry := range data.Entries {
			fmt.Fprintf(&text, "%s %s %s\n", entry.Classification, entry.Symbol, entry.Detail)
		}
	case map[string]string:
		fmt.Fprintf(&text, "otelplan %s\nGo %s\n", data["otelplan"], data["go"])
	}
	_, err := io.WriteString(out, text.String())
	return err
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
