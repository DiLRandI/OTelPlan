package compiler

import (
	"errors"
	"fmt"
	"go/version"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/modfile"
)

var (
	errInvalidWorkspaceAnalysis         = errors.New("workspace preparation requires analyzed root and Go version")
	errAnalyzedWorkspaceConfiguration   = errors.New("invalid analyzed workspace configuration")
	errWorkspaceAnalysisMismatch        = errors.New("workspace identity disagrees with analysis")
	errMissingAnalyzedMainModule        = errors.New("analysis root has no main module")
	errInvalidAnalyzedAlternateManifest = errors.New("invalid analyzed alternate module manifest")
)

// WorkspaceForAnalysis describes the workspace used by discovery. Runtime and
// Parent are supplied by the caller before preparing disposable module copies.
func WorkspaceForAnalysis(code *model.CodeModel) (WorkspaceRequest, error) {
	var empty WorkspaceRequest

	if code == nil || !filepath.IsAbs(code.ModuleRoot) || !version.IsValid(code.EffectiveBuild.GoVersion) {
		return empty, errInvalidWorkspaceAnalysis
	}

	if code.EffectiveBuild.Workspace {
		return workspaceFromExistingAnalysis(code)
	}

	if code.WorkspaceFile != "" {
		return empty, errWorkspaceAnalysisMismatch
	}

	return workspaceFromModuleAnalysis(code)
}

func workspaceFromExistingAnalysis(code *model.CodeModel) (WorkspaceRequest, error) {
	var request WorkspaceRequest

	if !filepath.IsAbs(code.WorkspaceFile) || code.EffectiveBuild.ModFile != "" {
		return request, errAnalyzedWorkspaceConfiguration
	}

	data, err := readAnalyzedWorkspaceInput(code.WorkspaceFile)
	if err != nil {
		return request, fmt.Errorf("read analyzed workspace manifest: %w", err)
	}

	_, err = modfile.ParseWork("go.work", data, nil)
	if err != nil {
		return request, fmt.Errorf("invalid analyzed workspace manifest: %w", err)
	}

	sums, err := readAnalyzedWorkspaceInput(code.WorkspaceFile + ".sum")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return request, fmt.Errorf("read analyzed workspace checksums: %w", err)
	}

	request.OriginalWorkspaceDir = filepath.Dir(code.WorkspaceFile)
	request.Workspace, request.WorkspaceSums = data, sums

	return request, nil
}

func readAnalyzedWorkspaceInput(filename string) ([]byte, error) {
	resolved, err := filepath.EvalSymlinks(filename)
	if err != nil {
		return nil, fmt.Errorf("resolve analyzed workspace input: %w", err)
	}

	root, err := os.OpenRoot(filepath.Dir(resolved))
	if err != nil {
		return nil, fmt.Errorf("open analyzed workspace input directory: %w", err)
	}

	defer func() { _ = root.Close() }()

	data, err := root.ReadFile(filepath.Base(resolved))
	if err != nil {
		return nil, fmt.Errorf("read analyzed workspace input: %w", err)
	}

	return data, nil
}

func analyzedMainModuleDirectory(code *model.CodeModel) (string, error) {
	directory := ""
	for _, module := range code.Modules {
		if module.Main && filepath.IsAbs(module.Dir) && withinTree(module.Dir, code.ModuleRoot) &&
			len(module.Dir) > len(directory) {
			directory = filepath.Clean(module.Dir)
		}
	}

	if directory == "" {
		return "", errMissingAnalyzedMainModule
	}

	return directory, nil
}

func workspaceFromModuleAnalysis(code *model.CodeModel) (WorkspaceRequest, error) {
	var request WorkspaceRequest

	moduleDir, err := analyzedMainModuleDirectory(code)
	if err != nil {
		return request, err
	}

	syntax := new(modfile.FileSyntax)
	syntax.Name = "go.work"
	work := new(modfile.WorkFile)
	work.Syntax = syntax

	err = work.AddGoStmt(strings.TrimPrefix(code.EffectiveBuild.GoVersion, "go"))
	if err != nil {
		return request, fmt.Errorf("invalid analyzed Go version: %w", err)
	}

	err = work.AddUse(moduleDir, "")
	if err != nil {
		return request, fmt.Errorf("add analyzed application module: %w", err)
	}

	if code.EffectiveBuild.ModFile != "" {
		if !filepath.IsAbs(code.EffectiveBuild.ModFile) || !strings.HasSuffix(code.EffectiveBuild.ModFile, ".mod") {
			return request, errInvalidAnalyzedAlternateManifest
		}

		request.AlternateModFiles = map[string]string{moduleDir: code.EffectiveBuild.ModFile}
	}

	request.OriginalWorkspaceDir = moduleDir
	request.Workspace = modfile.Format(work.Syntax)

	return request, nil
}
