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

type buildFile struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type buildSummary struct {
	Files   []buildFile       `json:"files,omitempty"`
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
	directoryOutput := strings.HasSuffix(args.Output, "/") || strings.HasSuffix(args.Output, "\\")
	if info, err := os.Stat(destination); err == nil && info.IsDir() {
		directoryOutput = true
	}
	if !directoryOutput && protectedBuildOutput(destination, opts, code) {
		return fail(2, model.CodeInvalidPolicy, "build output must not replace Go source or module files")
	}
	built, err := compiler.BuildResolved(context.Background(), compiler.ResolvedBuildRequest{Code: code, Plan: plan, Backend: backend, Executable: executable, RuntimeVersion: Version, Env: os.Environ(), GoArgs: args.GoArgs, Offline: opts.offline, DefaultOutput: destination == "", DirectoryOutput: directoryOutput, Packages: args.Packages})
	if err != nil {
		return fail(8, model.CodeCompilationFailed, "isolated backend build failed")
	}
	defer func() { _ = os.RemoveAll(built.Dir) }()

	result := buildSummary{Backend: backend}
	destinations := make([]string, len(built.Files))
	for i, artifact := range built.Files {
		target := destination
		if target == "" {
			target = filepath.Join(opts.root, artifact.DefaultName)
		} else if directoryOutput {
			target = filepath.Join(target, artifact.DefaultName)
		}
		target, err = filepath.Abs(target)
		if err != nil {
			return fail(1, model.CodeArtifactOutput, "cannot resolve build output")
		}
		if protectedBuildOutput(target, opts, code) {
			return fail(2, model.CodeInvalidPolicy, "build output must not replace source or project metadata")
		}
		if info, err := os.Lstat(target); err == nil && !info.Mode().IsRegular() {
			return fail(1, model.CodeArtifactOutput, "build output cannot replace a directory or symlink")
		} else if err != nil && !os.IsNotExist(err) {
			return fail(1, model.CodeArtifactOutput, "cannot inspect build output")
		}
		destinations[i] = target
	}
	for i, artifact := range built.Files {
		if err := compiler.PublishBuildArtifact(artifact, destinations[i]); err != nil {
			return fail(1, model.CodeArtifactOutput, "cannot publish verified build output")
		}
		if directoryOutput {
			result.Files = append(result.Files, buildFile{Path: destinations[i], Digest: artifact.Digest})
		} else {
			result.Path, result.Digest = destinations[i], artifact.Digest
		}
	}
	return result, 0, nil
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
