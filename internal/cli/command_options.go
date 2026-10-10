package cli

import (
	"errors"
	"io"
)

var (
	errCheckDryRunConflict = errors.New("cannot combine --check and --dry-run")
	errScanOnlyOptions     = errors.New("--dependencies, --interfaces, and --calls are supported by scan")
	errPolicyOnlyOptions   = errors.New("--config, --strict, and --allow-large-plan require a policy command")
	errEmptyCompileOutput  = errors.New("--output must not be empty")
	errOutputApplicability = errors.New("--output is supported by init and compile")
	errCleanApplicability  = errors.New("--clean is supported by compile")
	errInitApplicability   = errors.New("--force, --interactive, and --non-interactive are supported by init")
	errInteractiveConflict = errors.New("cannot combine --interactive and --non-interactive")
	errInteractiveJSON     = errors.New("--interactive requires text output")
	errInteractiveInput    = errors.New("--interactive requires an input stream")
	errCheckApplicability  = errors.New("--check is supported by lock and diff")
	errDryRunApplicability = errors.New("--dry-run is supported by lock")
)

func validateCommandOptions(command string, opts options, stdin io.Reader) error {
	if opts.check && opts.dryRun {
		return errCheckDryRunConflict
	}

	err := validateAnalysisOptions(command, opts)
	if err != nil {
		return err
	}

	err = validateArtifactOptions(command, opts)
	if err != nil {
		return err
	}

	err = validateInitializationOptions(command, opts, stdin)
	if err != nil {
		return err
	}

	return validateLockOptions(command, opts)
}

func validateAnalysisOptions(command string, opts options) error {
	if (opts.dependencies || opts.interfaces || opts.callGraph) && command != "scan" {
		return errScanOnlyOptions
	}

	if (opts.strict || opts.allowLargePlan || opts.configSet) && !isPolicyCommand(command) {
		return errPolicyOnlyOptions
	}

	return nil
}

func isPolicyCommand(command string) bool {
	switch command {
	case inspectCommandName, explainCommandName, validateCommandName, lockCommandName,
		diffCommandName, compileCommandName, buildCommandName:
		return true
	default:
		return false
	}
}

func validateArtifactOptions(command string, opts options) error {
	if command == compileCommandName && opts.output == "" {
		return errEmptyCompileOutput
	}

	if opts.outputSet && command != compileCommandName && command != initCommandName {
		return errOutputApplicability
	}

	if opts.clean && command != compileCommandName {
		return errCleanApplicability
	}

	return nil
}

func validateInitializationOptions(command string, opts options, stdin io.Reader) error {
	if (opts.force || opts.interactive || opts.nonInteractive) && command != initCommandName {
		return errInitApplicability
	}

	return validateInteractiveOptions(opts, stdin)
}

func validateInteractiveOptions(opts options, stdin io.Reader) error {
	if opts.interactive && opts.nonInteractive {
		return errInteractiveConflict
	}

	if opts.interactive && opts.format == jsonFormat {
		return errInteractiveJSON
	}

	if opts.interactive && stdin == nil {
		return errInteractiveInput
	}

	return nil
}

func validateLockOptions(command string, opts options) error {
	if opts.check && command != lockCommandName && command != diffCommandName {
		return errCheckApplicability
	}

	if opts.dryRun && command != lockCommandName {
		return errDryRunApplicability
	}

	return nil
}
