package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

type compileSummary struct {
	Path    string            `json:"path"`
	Files   int               `json:"files"`
	Backend model.LockBackend `json:"backend"`
}

const exitCompilation = 8

func compileCommand(ctx context.Context, opts options, policy *model.Policy, code *model.CodeModel,
	plan model.ResolvedPlan) (any, int, model.DiagnosticErrorList) {
	executable, err := exec.LookPath("otelc")
	if err != nil {
		recordFailure(opts, model.CodeBackendUnsupported, "find pinned backend", err)

		return compileFailure(exitBackend, "cannot find pinned otelc executable on PATH")
	}

	backend, err := otelc.VerifyExecutable(ctx, executable, policy.Backend.Version)
	if err != nil {
		recordFailure(opts, model.CodeBackendUnsupported, "verify pinned backend", err)

		return compileFailure(exitBackend, "backend executable does not match the pinned version")
	}

	files, err := otelc.RenderBundle(backend, Version, code, plan, "otelplan.local/generated")
	if err != nil {
		recordFailure(opts, model.CodeBackendUnsupported, "render backend bundle", err)

		return compileFailure(exitBackend, err.Error())
	}

	staged, err := compiler.StageArtifacts("", files)
	if err != nil {
		recordFailure(opts, model.CodeArtifactOutput, "stage generated artifacts", err)

		return compileFailure(1, "cannot stage compiler artifacts")
	}

	defer func() { _ = os.RemoveAll(staged.Dir) }()

	diagnostics := validateCompiledRuntime(ctx, opts, code.EffectiveBuild, staged)
	if diagnostics != nil {
		return nil, exitCompilation, diagnostics
	}

	destination := opts.output
	if !filepath.IsAbs(destination) {
		destination = filepath.Join(opts.root, destination)
	}

	published, err := compiler.PublishArtifacts(destination, files, opts.clean)
	if err != nil {
		recordFailure(opts, model.CodeArtifactOutput, "publish generated artifacts", err)

		return compileFailure(1, err.Error())
	}

	return compileSummary{Path: published.Dir, Files: len(published.Files), Backend: backend}, 0, nil
}

func validateCompiledRuntime(ctx context.Context, opts options, build model.BuildEnvironment,
	staged model.Artifacts) model.DiagnosticErrorList {
	env, buildFlags, err := compiler.RecordedBuildEnvironment(os.Environ(), build)
	if err != nil {
		recordFailure(opts, model.CodeCompilationFailed, "restore analyzed build environment", err)

		return compileDiagnostic(model.CodeCompilationFailed, "cannot restore analyzed Go environment")
	}

	args := append([]string{"test", "-mod=readonly"}, buildFlags...)
	args = append(args, "./...")
	command := exec.CommandContext(ctx, "go", args...)
	command.Dir = staged.Dir

	env = append(env, "GOWORK=off")
	command.Env = env

	if opts.offline {
		command.Env = append(command.Env, "GOPROXY=off", "GONOPROXY=none", "GOSUMDB=off", "GOTOOLCHAIN=local")
	}

	err = command.Run()
	if err != nil {
		recordFailure(opts, model.CodeCompilationFailed, "compile generated runtime", err)

		return compileDiagnostic(model.CodeCompilationFailed, "generated source compilation failed")
	}

	err = compiler.VerifyArtifacts(staged)
	if err != nil {
		recordFailure(opts, model.CodeCompilationFailed, "verify generated artifacts", err)

		return compileDiagnostic(model.CodeCompilationFailed, "generated source verification changed artifacts")
	}

	return nil
}

func compileFailure(exit int, message string) (any, int, model.DiagnosticErrorList) {
	code := model.CodeBackendUnsupported

	switch exit {
	case exitCompilation:
		code = model.CodeCompilationFailed
	case 1:
		code = model.CodeArtifactOutput
	}

	return nil, exit, compileDiagnostic(code, message)
}

func compileDiagnostic(code model.Code, message string) model.DiagnosticErrorList {
	var diagnostic model.DiagnosticError

	diagnostic.Severity = model.SeverityError
	diagnostic.Code = code
	diagnostic.Message = message

	return model.DiagnosticErrorList{diagnostic}
}
