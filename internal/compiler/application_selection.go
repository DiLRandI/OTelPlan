package compiler

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/modfile"
)

const applicationSelectionFileMode = 0o600

var errMissingSelectionRuntime = errors.New("prepared workspace has no generated runtime")

func applicationModuleSelection(ctx context.Context, workspace PreparedWorkspace, dir string,
	env []string) ([]model.ModuleInfo, error) {
	err := ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("read application module baseline: %w", err)
	}

	root, err := os.OpenRoot(workspace.Dir)
	if err != nil {
		return nil, fmt.Errorf("open prepared application workspace: %w", err)
	}

	defer func() { _ = root.Close() }()

	originalName, err := filepath.Rel(workspace.Dir, workspace.WorkspaceFile)
	if err != nil {
		return nil, fmt.Errorf("locate prepared workspace manifest: %w", err)
	}

	data, err := root.ReadFile(originalName)
	if err != nil {
		return nil, fmt.Errorf("read prepared workspace manifest: %w", err)
	}

	isolated, err := applicationWorkspaceManifest(data, workspace.Runtime.Dir)
	if err != nil {
		return nil, err
	}

	name := "application-" + rand.Text() + ".work"

	err = writeSelectionFile(root, name, isolated)
	if err != nil {
		return nil, err
	}

	defer func() { _ = root.Remove(name) }()

	err = copySelectionChecksums(root, originalName, name)
	if err != nil {
		return nil, err
	}

	defer func() { _ = root.Remove(name + ".sum") }()

	env = append(append([]string(nil), env...), "GOWORK="+filepath.Join(workspace.Dir, name))

	return ReadModuleSelection(ctx, dir, env)
}

func applicationWorkspaceManifest(data []byte, runtimeDirectory string) ([]byte, error) {
	work, err := modfile.ParseWork("go.work", data, nil)
	if err != nil {
		return nil, fmt.Errorf("parse prepared application workspace: %w", err)
	}

	found := false

	for _, use := range work.Use {
		if filepath.Clean(use.Path) == filepath.Clean(runtimeDirectory) {
			found = true
		}
	}

	if !found {
		return nil, errMissingSelectionRuntime
	}

	err = work.DropUse(runtimeDirectory)
	if err != nil {
		return nil, fmt.Errorf("remove generated runtime from application baseline: %w", err)
	}

	work.Cleanup()

	return modfile.Format(work.Syntax), nil
}

func writeSelectionFile(root *os.Root, name string, data []byte) error {
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, applicationSelectionFileMode)
	if err != nil {
		return fmt.Errorf("create isolated application workspace input: %w", err)
	}

	complete := false

	defer func() {
		_ = file.Close()

		if !complete {
			_ = root.Remove(name)
		}
	}()

	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("write isolated application workspace input: %w", err)
	}

	err = file.Close()
	if err != nil {
		return fmt.Errorf("close isolated application workspace input: %w", err)
	}

	complete = true

	return nil
}

func copySelectionChecksums(root *os.Root, originalName, isolatedName string) error {
	sums, err := root.ReadFile(originalName + ".sum")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("read prepared workspace checksums: %w", err)
	}

	err = writeSelectionFile(root, isolatedName+".sum", sums)
	if err != nil {
		return fmt.Errorf("copy isolated application workspace checksums: %w", err)
	}

	return nil
}
