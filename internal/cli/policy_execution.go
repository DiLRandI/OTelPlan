package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/internal/policy"
	"github.com/DiLRandI/OTelPlan/internal/resolve"
	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const exitInvalidPolicy = 3

var (
	errExplainArguments = errors.New("explain requires one canonical symbol")
	errPolicyArguments  = errors.New("takes no positional arguments")
)

type policyExecution struct {
	policy     *model.Policy
	inventory  *model.CodeModel
	resolution resolve.Result
	build      buildArguments
}

func runPolicyCommand(ctx context.Context, command string, opts options,
	args []string) (any, int, model.DiagnosticErrorList) {
	buildArgs, err := policyCommandArguments(command, args)
	if err != nil {
		return nil, exitUsage, compileDiagnostic(model.CodeInvalidPolicy, err.Error())
	}

	loaded, diagnostics := loadCommandPolicy(opts)
	if diagnostics.HasErrors() {
		return nil, exitInvalidPolicy, diagnostics
	}

	inventory, analysisDiagnostics := loadPolicyInventory(ctx, opts, loaded, buildArgs)
	if analysisDiagnostics.HasErrors() {
		diagnostics = append(diagnostics, analysisDiagnostics...)

		return nil, exitAnalysis, diagnostics
	}

	execution := policyExecution{
		policy: loaded, inventory: inventory, resolution: resolve.Resolve(loaded, inventory), build: buildArgs,
	}
	diagnostics, exit := validatePolicyExecution(opts, execution)

	switch command {
	case inspectCommandName:
		return previewPlan(execution.resolution.Plan), exit, diagnostics
	case explainCommandName:
		return execution.explain(args[0], exit, diagnostics)
	default:
		if exit != 0 {
			return nil, exit, diagnostics
		}

		data, commandExit, commandDiagnostics := execution.execute(ctx, command, opts)
		diagnostics = append(diagnostics, commandDiagnostics...)

		return data, commandExit, diagnostics
	}
}

func policyCommandArguments(command string, args []string) (buildArguments, error) {
	var empty buildArguments

	if command == explainCommandName && len(args) != 1 {
		return empty, errExplainArguments
	}

	if command != explainCommandName && command != buildCommandName && len(args) != 0 {
		return empty, fmt.Errorf("%s %w", command, errPolicyArguments)
	}

	if command == buildCommandName {
		return parseBuildArguments(args)
	}

	return empty, nil
}

func loadCommandPolicy(opts options) (*model.Policy, model.DiagnosticErrorList) {
	config := opts.config
	if !filepath.IsAbs(config) {
		config = filepath.Join(opts.root, config)
	}

	loaded, err := policy.Load(config)
	if err != nil {
		recordFailure(opts, model.CodeInvalidPolicy, "load policy", err)

		return nil, compileDiagnostic(model.CodeInvalidPolicy, "cannot read or parse policy at "+config)
	}

	return loaded, policy.Validate(loaded)
}

func loadPolicyInventory(ctx context.Context, opts options, loaded *model.Policy,
	buildArgs buildArguments) (*model.CodeModel, model.DiagnosticErrorList) {
	var analysis discovery.Options

	analysis.Root, analysis.Patterns = opts.root, loaded.Project.Packages
	analysis.BuildFlags, analysis.BuildTags = buildArgs.AnalysisFlags, loaded.Project.BuildTags
	analysis.IncludeTests, analysis.IncludeDependencies = loaded.Project.IncludeTests, loaded.Project.IncludeDependencies
	analysis.Offline = opts.offline

	inventory, err := discovery.LoadContext(ctx, analysis)
	if err != nil {
		recordFailure(opts, model.CodeUnresolvedSymbol, "analyze policy packages", err)

		return nil, compileDiagnostic(model.CodeUnresolvedSymbol, err.Error())
	}

	recordBuildContext(opts, inventory.EffectiveBuild)

	return inventory, nil
}

func validatePolicyExecution(opts options, execution policyExecution) (model.DiagnosticErrorList, int) {
	var safetyOptions validate.Options

	safetyOptions.AllowLargePlan = opts.allowLargePlan
	diagnostics := execution.resolution.Diagnostics
	diagnostics = append(diagnostics, validate.Safety(execution.inventory, execution.resolution.Plan, safetyOptions)...)

	if diagnostics == nil {
		diagnostics = model.DiagnosticErrorList{}
	}

	backendDiagnostics := otelc.Check(execution.policy.Backend.Version, execution.inventory, execution.resolution.Plan)
	diagnostics = append(diagnostics, backendDiagnostics...)

	return diagnostics, policyValidationExit(opts, diagnostics, backendDiagnostics)
}

func policyValidationExit(opts options, diagnostics, backendDiagnostics model.DiagnosticErrorList) int {
	if backendDiagnostics.HasErrors() {
		return exitBackend
	}

	if diagnostics.HasErrors() || (opts.strict && len(diagnostics.Warnings()) > 0) {
		return exitValidation
	}

	return 0
}

func (execution policyExecution) explain(symbol string, exit int,
	diagnostics model.DiagnosticErrorList) (any, int, model.DiagnosticErrorList) {
	for _, explanation := range execution.resolution.Explanations {
		if string(explanation.SymbolID) == symbol {
			return explanation, exit, diagnostics
		}
	}

	diagnostics = append(diagnostics, compileDiagnostic(model.CodeUnresolvedSymbol,
		"symbol does not exist or policy could not be resolved")...)

	return nil, exitValidation, diagnostics
}

func (execution policyExecution) execute(ctx context.Context, command string,
	opts options) (any, int, model.DiagnosticErrorList) {
	switch command {
	case buildCommandName:
		return buildCommand(ctx, opts, execution.build, execution.policy, execution.inventory, execution.resolution.Plan)
	case compileCommandName:
		return compileCommand(ctx, opts, execution.policy, execution.inventory, execution.resolution.Plan)
	default:
		return lockCommand(command, opts, execution.policy, execution.inventory, execution.resolution.Plan)
	}
}
