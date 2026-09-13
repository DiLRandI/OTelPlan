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

func compileCommand(opts options, p *model.Policy, code *model.CodeModel, plan model.ResolvedPlan) (any, int, model.DiagnosticList) {
	fail := func(exit int, message string) (any, int, model.DiagnosticList) {
		code := model.CodeBackendUnsupported
		if exit == 8 {
			code = model.CodeCompilationFailed
		}
		if exit == 1 {
			code = model.CodeArtifactOutput
		}
		return nil, exit, model.DiagnosticList{{Severity: model.SeverityError, Code: code, Message: message}}
	}
	executable, err := exec.LookPath("otelc")
	if err != nil {
		return fail(7, "cannot find pinned otelc executable on PATH")
	}
	backend, err := otelc.VerifyExecutable(context.Background(), executable, p.Backend.Version)
	if err != nil {
		return fail(7, "backend executable does not match the pinned version")
	}
	files, err := otelc.RenderBundle(backend, Version, code, plan, "otelplan.local/generated")
	if err != nil {
		return fail(7, err.Error())
	}
	staged, err := compiler.StageArtifacts("", files)
	if err != nil {
		return fail(1, "cannot stage compiler artifacts")
	}
	defer func() { _ = os.RemoveAll(staged.Dir) }()
	env, buildFlags, err := compiler.RecordedBuildEnvironment(os.Environ(), code.EffectiveBuild)
	if err != nil {
		return fail(8, "cannot restore analyzed Go environment")
	}
	args := append([]string{"test", "-mod=readonly"}, buildFlags...)
	command := exec.Command("go", append(args, "./...")...)
	command.Dir = staged.Dir
	command.Env = append(env, "GOWORK=off")
	if opts.offline {
		command.Env = append(command.Env, "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	}
	if err := command.Run(); err != nil {
		return fail(8, "generated source compilation failed")
	}
	if err := compiler.VerifyArtifacts(staged); err != nil {
		return fail(8, "generated source verification changed artifacts")
	}
	destination := opts.output
	if !filepath.IsAbs(destination) {
		destination = filepath.Join(opts.root, destination)
	}
	published, err := compiler.PublishArtifacts(destination, files, opts.clean)
	if err != nil {
		return fail(1, err.Error())
	}
	return compileSummary{Path: published.Dir, Files: len(published.Files), Backend: backend}, 0, nil
}
