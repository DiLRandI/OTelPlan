package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/DiLRandI/OTelPlan/internal/validate"
)

var (
	errUnknownGlobalFlag      = errors.New("unknown flag")
	errGlobalFlagValueMissing = errors.New("requires a value")
	errInvalidGlobalFlag      = errors.New("invalid flag value or syntax")
	errInvalidOutputFormat    = errors.New("format must be text or json")
)

type globalArguments struct {
	flags       []string
	positionals []string
	format      string
	err         error
}

func parse(args []string) (options, []string, error) {
	var opts options

	flags := globalFlags(&opts)
	arguments := collectGlobalArguments(flags, args, opts.format)
	err := flags.Parse(arguments.flags)

	if err != nil && arguments.err == nil {
		arguments.err = errInvalidGlobalFlag
	}

	opts.format = arguments.format
	if arguments.err != nil {
		return opts, arguments.positionals, arguments.err
	}

	if opts.format != "text" && opts.format != jsonFormat {
		return opts, arguments.positionals, errInvalidOutputFormat
	}

	opts.recordExplicitFlags(flags)

	return opts, arguments.positionals, nil
}

func (opts *options) recordExplicitFlags(flags *flag.FlagSet) {
	flags.Visit(func(option *flag.Flag) {
		switch option.Name {
		case "output":
			opts.outputSet = true
		case "config":
			opts.configSet = true
		case "cache-dir":
			opts.cacheSet = true
		case "warn-targets", "strong-warn-targets":
			opts.targetLimitsSet = true
		case "max-targets":
			opts.targetLimitsSet, opts.maximumTargetsSet = true, true
		}
	})
}

func globalFlags(opts *options) *flag.FlagSet {
	flags := flag.NewFlagSet("otelplan", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.output, "output", ".otelplan/build", "artifact output relative to root")
	flags.BoolVar(&opts.clean, "clean", false, "replace verified artifact output")
	flags.BoolVar(&opts.force, "force", false, "replace an existing starter policy")
	flags.BoolVar(&opts.interactive, "interactive", false, "review starter suggestions one by one")
	flags.BoolVar(&opts.nonInteractive, "non-interactive", false, "generate a starter policy without prompts")
	flags.StringVar(&opts.root, "root", ".", "project root")
	flags.StringVar(&opts.config, "config", "otelplan.yaml", "policy path relative to root")
	flags.StringVar(&opts.cacheDir, "cache-dir", "", "private semantic cache path relative to root")
	flags.StringVar(&opts.format, "format", "text", "text or json")
	flags.BoolVar(&opts.strict, "strict", false, "fail on warnings")
	flags.BoolVar(&opts.offline, "offline", false, "disable Go network resolution")
	flags.BoolVar(&opts.check, "check", false, "check without writing")
	flags.BoolVar(&opts.dryRun, "dry-run", false, "preview without writing")
	flags.BoolVar(&opts.allowLargePlan, "allow-large-plan", false, "acknowledge large target count")
	flags.IntVar(&opts.warningTargets, "warn-targets", validate.DefaultWarningTargets, "target-count warning threshold")
	flags.IntVar(&opts.strongWarningTargets, "strong-warn-targets", validate.DefaultStrongWarningTargets,
		"strong target-count warning threshold")
	flags.IntVar(&opts.maximumTargets, "max-targets", validate.DefaultMaximumTargets, "binding maximum target count")
	flags.BoolVar(&opts.help, "help", false, "show usage")
	flags.BoolVar(&opts.help, "h", false, "show usage")
	flags.BoolVar(&opts.quiet, "quiet", false, "suppress informational text")
	flags.BoolVar(&opts.verbose, "verbose", false, "include safe error causes and build context")
	flags.BoolVar(&opts.noColor, "no-color", false, "disable color")
	flags.BoolVar(&opts.dependencies, "dependencies", false, "include dependency code in scan")
	flags.BoolVar(&opts.callGraph, "calls", false, "include conservative advisory calls in scan")
	flags.BoolVar(&opts.interfaces, "interfaces", false, "show interface methods in text scans")

	return flags
}

func collectGlobalArguments(flags *flag.FlagSet, args []string, format string) globalArguments {
	var arguments globalArguments

	arguments.format = format
	remaining := args

	for len(remaining) > 0 {
		argument := remaining[0]
		remaining = remaining[1:]

		if argument == "--" {
			arguments.positionals = append(arguments.positionals, remaining...)

			break
		}

		if !strings.HasPrefix(argument, "-") || argument == "-" {
			arguments.positionals = append(arguments.positionals, argument)

			continue
		}

		tail, err := arguments.appendFlag(flags, argument, remaining)
		remaining = tail

		if err != nil && arguments.err == nil {
			arguments.err = err
		}
	}

	return arguments
}

func (arguments *globalArguments) appendFlag(flags *flag.FlagSet, argument string,
	remaining []string) ([]string, error) {
	name, value, hasValue := strings.Cut(strings.TrimLeft(argument, "-"), "=")
	option := flags.Lookup(name)

	if option == nil {
		return remaining, fmt.Errorf("%w: --%s", errUnknownGlobalFlag, name)
	}

	arguments.flags = append(arguments.flags, argument)

	if !hasValue && !isBooleanGlobalFlag(option) {
		if len(remaining) == 0 {
			return remaining, fmt.Errorf("flag --%s %w", name, errGlobalFlagValueMissing)
		}

		value, remaining = remaining[0], remaining[1:]
		arguments.flags = append(arguments.flags, value)
	}

	if name == "format" {
		arguments.format = value
	}

	return remaining, nil
}

func isBooleanGlobalFlag(option *flag.Flag) bool {
	boolean, ok := option.Value.(interface{ IsBoolFlag() bool })

	return ok && boolean.IsBoolFlag()
}
