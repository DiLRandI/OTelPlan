package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

type buildSummary struct {
	Path    string            `json:"path,omitempty"`
	Digest  string            `json:"digest,omitempty"`
	Backend model.LockBackend `json:"backend"`
}

func buildCommand(opts options, args buildArguments, p *model.Policy, code *model.CodeModel, plan model.ResolvedPlan) (any, int, model.DiagnosticList) {
	fail := func(exit int, code model.Code, message string) (any, int, model.DiagnosticList) {
		return nil, exit, model.DiagnosticList{{Severity: model.SeverityError, Code: code, Message: message}}
	}
	if code.EffectiveBuild.ModuleMode == "vendor" {
		return fail(7, model.CodeBackendUnsupported, "vendor-mode build preparation is not implemented")
	}
	executable, err := exec.LookPath("otelc")
	if err != nil {
		return fail(7, model.CodeBackendUnsupported, "cannot find pinned otelc executable on PATH")
	}
	backend, err := otelc.VerifyExecutable(context.Background(), executable, p.Backend.Version)
	if err != nil {
		return fail(7, model.CodeBackendVersionMismatch, "backend executable does not match pinned version")
	}
	destination := args.Output
	if destination != "" && !filepath.IsAbs(destination) {
		destination = filepath.Join(opts.root, destination)
	}
	if protectedBuildOutput(destination, opts, code) {
		return fail(2, model.CodeInvalidPolicy, "build output must not replace Go source or module files")
	}
	built, err := compiler.BuildResolved(context.Background(), compiler.ResolvedBuildRequest{Code: code, Plan: plan, Backend: backend, Executable: executable, RuntimeVersion: Version, Env: os.Environ(), GoArgs: args.GoArgs, Offline: opts.offline, DefaultOutput: destination == "", Packages: args.Packages})
	if err != nil {
		return fail(8, model.CodeCompilationFailed, "isolated backend build failed")
	}
	defer func() { _ = os.RemoveAll(built.Dir) }()
	if built.File == "" {
		return buildSummary{Backend: backend}, 0, nil
	}
	if destination == "" {
		destination = filepath.Join(opts.root, built.DefaultName)
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return fail(1, model.CodeArtifactOutput, "cannot resolve build output")
	}
	if protectedBuildOutput(destination, opts, code) {
		return fail(2, model.CodeInvalidPolicy, "build output must not replace source or project metadata")
	}
	if err := compiler.PublishBuildArtifact(built, destination); err != nil {
		return fail(1, model.CodeArtifactOutput, "cannot publish verified build output")
	}
	return buildSummary{Path: destination, Digest: built.Digest, Backend: backend}, 0, nil
}

func protectedBuildOutput(destination string, opts options, code *model.CodeModel) bool {
	if destination == "" {
		return false
	}
	name := filepath.Base(destination)
	if strings.HasSuffix(name, ".go") || name == "go.mod" || name == "go.sum" || name == "go.work" || name == "go.work.sum" {
		return true
	}
	config := opts.config
	if !filepath.IsAbs(config) {
		config = filepath.Join(opts.root, config)
	}
	destination, err := filepath.Abs(destination)
	if err != nil {
		return true
	}
	for _, original := range []string{config, filepath.Join(opts.root, "otelplan.lock"), code.EffectiveBuild.ModFile, code.WorkspaceFile} {
		if original == "" {
			continue
		}
		absolute, err := filepath.Abs(original)
		if err == nil && absolute == destination {
			return true
		}
	}
	return false
}
