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

type buildDestination struct {
	path      string
	directory bool
}

func buildCommand(ctx context.Context, opts options, args buildArguments, policy *model.Policy,
	code *model.CodeModel, plan model.ResolvedPlan) (any, int, model.DiagnosticErrorList) {
	executable, err := exec.LookPath("otelc")
	if err != nil {
		recordFailure(opts, model.CodeBackendUnsupported, "find pinned backend", err)

		return buildFailure(exitBackend, model.CodeBackendUnsupported, "cannot find pinned otelc executable on PATH")
	}

	backend, err := otelc.VerifyExecutable(ctx, executable, policy.Backend.Version)
	if err != nil {
		recordFailure(opts, model.CodeBackendVersionMismatch, "verify pinned backend", err)

		return buildFailure(exitBackend, model.CodeBackendVersionMismatch, "backend executable does not match pinned version")
	}

	destination, diagnostics := selectBuildDestination(opts, args.Output, code)
	if diagnostics != nil {
		return nil, exitUsage, diagnostics
	}

	built, err := compiler.BuildResolved(ctx, compiler.ResolvedBuildRequest{
		Code: code, Plan: plan, Backend: backend, Executable: executable, RuntimeVersion: Version,
		WorkingDir: "", Parent: "", Env: os.Environ(), GoArgs: args.GoArgs, Offline: opts.offline,
		DefaultOutput: destination.path == "", DirectoryOutput: destination.directory, Packages: args.Packages,
	})
	if err != nil {
		recordFailure(opts, model.CodeCompilationFailed, "build isolated workspace", err)

		return buildFailure(exitCompilation, model.CodeCompilationFailed, "isolated backend build failed")
	}

	defer func() { _ = os.RemoveAll(built.Dir) }()

	destinations, exit, diagnostics := destination.resolveAll(opts, code, built.Files)
	if diagnostics != nil {
		return nil, exit, diagnostics
	}

	return destination.publishAll(opts, backend, built.Files, destinations)
}

func selectBuildDestination(opts options, output string, code *model.CodeModel) (buildDestination,
	model.DiagnosticErrorList) {
	destination := output
	if destination != "" && !filepath.IsAbs(destination) {
		destination = filepath.Join(opts.root, destination)
	}

	directoryOutput := strings.HasSuffix(output, "/") || strings.HasSuffix(output, "\\")
	info, err := os.Stat(destination)

	if err == nil && info.IsDir() {
		directoryOutput = true
	}

	if !directoryOutput && protectedBuildOutput(destination, opts, code) {
		var invalid buildDestination

		return invalid, compileDiagnostic(model.CodeInvalidPolicy, "build output must not replace Go source or module files")
	}

	return buildDestination{path: destination, directory: directoryOutput}, nil
}

func (choice buildDestination) resolveAll(opts options, code *model.CodeModel,
	files []compiler.BuildArtifact) ([]string, int, model.DiagnosticErrorList) {
	destinations := make([]string, len(files))

	for artifactIndex, artifact := range files {
		target := choice.path
		if target == "" {
			target = filepath.Join(opts.root, artifact.DefaultName)
		} else if choice.directory {
			target = filepath.Join(target, artifact.DefaultName)
		}

		absolute, err := filepath.Abs(target)
		if err != nil {
			recordFailure(opts, model.CodeArtifactOutput, "resolve build output", err)

			return buildDestinationFailure(1, model.CodeArtifactOutput, "cannot resolve build output")
		}

		target = absolute

		if protectedBuildOutput(target, opts, code) {
			return buildDestinationFailure(exitUsage, model.CodeInvalidPolicy,
				"build output must not replace source or project metadata")
		}

		info, err := os.Lstat(target)
		if err == nil && !info.Mode().IsRegular() {
			return buildDestinationFailure(1, model.CodeArtifactOutput, "build output cannot replace a directory or symlink")
		} else if err != nil && !os.IsNotExist(err) {
			recordFailure(opts, model.CodeArtifactOutput, "inspect build output", err)

			return buildDestinationFailure(1, model.CodeArtifactOutput, "cannot inspect build output")
		}

		destinations[artifactIndex] = target
	}

	return destinations, 0, nil
}

func (choice buildDestination) publishAll(opts options, backend model.LockBackend, files []compiler.BuildArtifact,
	destinations []string) (any, int, model.DiagnosticErrorList) {
	var result buildSummary

	result.Backend = backend

	for artifactIndex, artifact := range files {
		err := compiler.PublishBuildArtifact(artifact, destinations[artifactIndex])
		if err != nil {
			recordFailure(opts, model.CodeArtifactOutput, "publish build output", err)

			return buildFailure(1, model.CodeArtifactOutput, "cannot publish verified build output")
		}

		if choice.directory {
			result.Files = append(result.Files, buildFile{Path: destinations[artifactIndex], Digest: artifact.Digest})
		} else {
			result.Path, result.Digest = destinations[artifactIndex], artifact.Digest
		}
	}

	return result, 0, nil
}

func buildDestinationFailure(exit int, code model.Code, message string) ([]string, int, model.DiagnosticErrorList) {
	return nil, exit, compileDiagnostic(code, message)
}

func buildFailure(exit int, code model.Code, message string) (any, int, model.DiagnosticErrorList) {
	return nil, exit, compileDiagnostic(code, message)
}

func protectedBuildOutput(destination string, opts options, code *model.CodeModel) bool {
	if destination == "" {
		return false
	}

	if reservedBuildOutputName(filepath.Base(destination)) {
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

	originals := []string{
		config, filepath.Join(opts.root, "otelplan.lock"), code.EffectiveBuild.ModFile, code.WorkspaceFile,
	}

	for _, original := range originals {
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

func reservedBuildOutputName(name string) bool {
	if strings.HasSuffix(name, ".go") {
		return true
	}

	switch name {
	case "go.mod", "go.sum", "go.work", "go.work.sum":
		return true
	default:
		return false
	}
}
