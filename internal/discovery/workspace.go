package discovery

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

func isolateWorkspace(filename string) (string, func(), error) {
	work, err := loadWorkspaceManifest(filename)
	if err != nil {
		return "", nil, err
	}

	directory, err := os.MkdirTemp("", "otelplan-workspace-*")
	if err != nil {
		return "", nil, fmt.Errorf("create isolated workspace directory: %w", err)
	}

	cleanup := func() { _ = os.RemoveAll(directory) }

	root, err := os.OpenRoot(directory)
	if err != nil {
		cleanup()

		return "", nil, fmt.Errorf("open isolated workspace directory: %w", err)
	}

	writeErr := writeWorkspaceMetadata(root, filename, modfile.Format(work.Syntax))

	closeErr := root.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close isolated workspace directory: %w", closeErr)
	}

	if writeErr != nil || closeErr != nil {
		cleanup()

		return "", nil, errors.Join(writeErr, closeErr)
	}

	return filepath.Join(directory, "go.work"), cleanup, nil
}

func loadWorkspaceManifest(filename string) (*modfile.WorkFile, error) {
	data, err := readBuildMetadata(filename)
	if err != nil {
		return nil, fmt.Errorf("read workspace: %w", err)
	}

	work, err := modfile.ParseWork(filename, data, nil)
	if err != nil {
		return nil, fmt.Errorf("parse workspace: %w", err)
	}

	base := filepath.Dir(filename)
	uses := make([]*modfile.Use, 0, len(work.Use))

	for _, use := range work.Use {
		uses = append(uses, &modfile.Use{
			Path: workspaceAbsolutePath(base, use.Path), ModulePath: use.ModulePath, Syntax: nil,
		})
	}

	work.SetUse(uses)

	for _, replacement := range work.Replace {
		if replacement.New.Version != "" {
			continue
		}

		err := work.AddReplace(replacement.Old.Path, replacement.Old.Version,
			workspaceAbsolutePath(base, replacement.New.Path), "")
		if err != nil {
			return nil, fmt.Errorf("resolve workspace replacement: %w", err)
		}
	}

	work.Cleanup()

	return work, nil
}

func workspaceAbsolutePath(base, name string) string {
	if filepath.IsAbs(name) {
		return name
	}

	return filepath.Join(base, name)
}

func writeWorkspaceMetadata(root *os.Root, original string, manifest []byte) error {
	err := root.WriteFile("go.work", manifest, isolatedManifestMode)
	if err != nil {
		return fmt.Errorf("write isolated workspace manifest: %w", err)
	}

	checksums, err := readBuildMetadata(original + ".sum")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read workspace checksums: %w", err)
	}

	if err == nil {
		writeErr := root.WriteFile("go.work.sum", checksums, isolatedManifestMode)
		if writeErr != nil {
			return fmt.Errorf("write isolated workspace checksums: %w", writeErr)
		}
	}

	return linkWorkspaceVendor(root, original)
}

func linkWorkspaceVendor(root *os.Root, original string) error {
	vendor := filepath.Join(filepath.Dir(original), "vendor")

	_, err := os.Stat(vendor)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("inspect workspace vendor directory: %w", err)
	}

	// Root.Symlink treats an outside directory as a file link on Windows.
	// The fixed link name is inside the owned private workspace directory.
	err = os.Symlink(vendor, filepath.Join(root.Name(), "vendor"))
	if err != nil {
		return fmt.Errorf("isolate workspace vendor directory: %w", err)
	}

	return nil
}
