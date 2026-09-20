package compiler

import (
	"errors"
	"go/version"
	"os"
	"path/filepath"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/modfile"
)

// WorkspaceForAnalysis describes the workspace used by discovery. Runtime and
// Parent are supplied by the caller before preparing disposable module copies.
func WorkspaceForAnalysis(code *model.CodeModel) (WorkspaceRequest, error) {
	if code == nil || !filepath.IsAbs(code.ModuleRoot) || !version.IsValid(code.EffectiveBuild.GoVersion) {
		return WorkspaceRequest{}, errors.New("workspace preparation requires analyzed root and Go version")
	}

	request := WorkspaceRequest{}

	if code.EffectiveBuild.Workspace {
		if !filepath.IsAbs(code.WorkspaceFile) || code.EffectiveBuild.ModFile != "" {
			return request, errors.New("invalid analyzed workspace configuration")
		}

		data, err := os.ReadFile(code.WorkspaceFile)
		if err != nil {
			return request, errors.New("read analyzed workspace manifest")
		}

		if _, err := modfile.ParseWork("go.work", data, nil); err != nil {
			return request, errors.New("invalid analyzed workspace manifest")
		}

		sums, err := os.ReadFile(code.WorkspaceFile + ".sum")
		if err != nil && !os.IsNotExist(err) {
			return request, errors.New("read analyzed workspace checksums")
		}

		request.OriginalWorkspaceDir = filepath.Dir(code.WorkspaceFile)
		request.Workspace, request.WorkspaceSums = data, sums

		return request, nil
	}

	if code.WorkspaceFile != "" {
		return request, errors.New("workspace identity disagrees with analysis")
	}

	moduleDir := ""
	for _, module := range code.Modules {
		if module.Main && filepath.IsAbs(module.Dir) && withinTree(module.Dir, code.ModuleRoot) && len(module.Dir) > len(moduleDir) {
			moduleDir = filepath.Clean(module.Dir)
		}
	}

	if moduleDir == "" {
		return request, errors.New("analysis root has no main module")
	}

	work := &modfile.WorkFile{Syntax: &modfile.FileSyntax{Name: "go.work"}}
	err := work.AddGoStmt(strings.TrimPrefix(code.EffectiveBuild.GoVersion, "go"))
	if err != nil {
		return request, errors.New("invalid analyzed Go version")
	}

	err = work.AddUse(moduleDir, "")
	if err != nil {
		return request, err
	}

	request.OriginalWorkspaceDir = moduleDir
	request.Workspace = modfile.Format(work.Syntax)

	if code.EffectiveBuild.ModFile != "" {
		if !filepath.IsAbs(code.EffectiveBuild.ModFile) || !strings.HasSuffix(code.EffectiveBuild.ModFile, ".mod") {
			return WorkspaceRequest{}, errors.New("invalid analyzed alternate module manifest")
		}

		request.AlternateModFiles = map[string]string{moduleDir: code.EffectiveBuild.ModFile}
	}

	return request, nil
}
