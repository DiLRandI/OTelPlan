package compiler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/modfile"
)

func applicationModuleSelection(ctx context.Context, workspace PreparedWorkspace, dir string, env []string) ([]model.ModuleInfo, error) {
	data, err := os.ReadFile(workspace.WorkspaceFile)
	if err != nil {
		return nil, err
	}
	work, err := modfile.ParseWork("go.work", data, nil)
	if err != nil {
		return nil, err
	}
	found := false
	for _, use := range work.Use {
		if filepath.Clean(use.Path) == filepath.Clean(workspace.Runtime.Dir) {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("prepared workspace has no generated runtime")
	}
	if err := work.DropUse(workspace.Runtime.Dir); err != nil {
		return nil, err
	}
	work.Cleanup()
	file, err := os.CreateTemp(workspace.Dir, "application-*.work")
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close(); _ = os.Remove(file.Name()); _ = os.Remove(file.Name() + ".sum") }()
	if _, err := file.Write(modfile.Format(work.Syntax)); err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	sums, err := os.ReadFile(workspace.WorkspaceFile + ".sum")
	if err == nil {
		if err := os.WriteFile(file.Name()+".sum", sums, 0o600); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return ReadModuleSelection(ctx, dir, append(append([]string(nil), env...), "GOWORK="+file.Name()))
}
