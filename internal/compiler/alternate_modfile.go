package compiler

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

const alternateModuleFileMode = 0o600

func installAlternateModuleFiles(alternate, copiedDir string) error {
	manifest, err := readWorkspaceSourceManifest(alternate)
	if err != nil {
		return fmt.Errorf("read alternate module manifest: %w", err)
	}

	sums, err := readWorkspaceSourceManifest(strings.TrimSuffix(alternate, ".mod") + ".sum")
	missingSums := errors.Is(err, fs.ErrNotExist)

	if err != nil && !missingSums {
		return fmt.Errorf("read alternate module checksums: %w", err)
	}

	root, err := os.OpenRoot(copiedDir)
	if err != nil {
		return fmt.Errorf("open isolated alternate module directory: %w", err)
	}

	defer func() { _ = root.Close() }()

	err = removeCopiedModuleFiles(root)
	if err != nil {
		return err
	}

	err = root.WriteFile("go.mod", manifest, alternateModuleFileMode)
	if err != nil {
		return fmt.Errorf("write isolated alternate manifest: %w", err)
	}

	if !missingSums {
		err := root.WriteFile("go.sum", sums, alternateModuleFileMode)
		if err != nil {
			return fmt.Errorf("write isolated alternate checksums: %w", err)
		}
	}

	return nil
}

func removeCopiedModuleFiles(root *os.Root) error {
	for _, name := range []string{"go.mod", "go.sum"} {
		err := root.Remove(name)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove copied module file: %w", err)
		}
	}

	return nil
}
