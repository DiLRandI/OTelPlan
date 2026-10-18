package cli

import (
	"errors"

	"github.com/DiLRandI/OTelPlan/internal/validate"
)

var (
	errTargetLimitApplicability = errors.New(
		"--warn-targets, --strong-warn-targets, and --max-targets require a policy command",
	)
	errPositiveTargetLimits = errors.New("target-count thresholds must be positive")
	errWarningTargetOrder   = errors.New("--warn-targets must not exceed --strong-warn-targets")
	errMaximumTargetConsent = errors.New("raising the maximum target count above the default requires --allow-large-plan")
)

func validateTargetLimitOptions(command string, opts options) error {
	if !opts.targetLimitsSet {
		return nil
	}

	if !isPolicyCommand(command) {
		return errTargetLimitApplicability
	}

	if opts.warningTargets <= 0 || opts.strongWarningTargets <= 0 || opts.maximumTargets <= 0 {
		return errPositiveTargetLimits
	}

	if opts.warningTargets > opts.strongWarningTargets {
		return errWarningTargetOrder
	}

	if opts.maximumTargetsSet && opts.maximumTargets > validate.DefaultMaximumTargets && !opts.allowLargePlan {
		return errMaximumTargetConsent
	}

	return nil
}

func targetSafetyOptions(opts options) validate.Options {
	var safety validate.Options

	safety.WarningTargets, safety.StrongWarningTargets = opts.warningTargets, opts.strongWarningTargets
	safety.MaximumTargets, safety.EnforceMaximum = opts.maximumTargets, opts.maximumTargetsSet
	safety.AllowLargePlan = opts.allowLargePlan

	return safety
}
