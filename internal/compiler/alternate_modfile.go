package compiler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func installAlternateModuleFiles(alternate, copiedDir string) error {
	manifest, err := os.ReadFile(alternate)
	if err != nil {
		return errors.New("read alternate module manifest")
	}

	sums, err := os.ReadFile(strings.TrimSuffix(alternate, ".mod") + ".sum")

	missingSums := os.IsNotExist(err)

	if err != nil && !missingSums {
		return errors.New("read alternate module checksums")
	}

	for _, name := range []string{"go.mod", "go.sum"} {
		err := os.Remove(filepath.Join(copiedDir, name))
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove copied module file: %w", err)
		}
	}

	if err := os.WriteFile(filepath.Join(copiedDir, "go.mod"), manifest, 0o600); err != nil {
		return fmt.Errorf("write isolated alternate manifest: %w", err)
	}

	if !missingSums {
		err := os.WriteFile(filepath.Join(copiedDir, "go.sum"), sums, 0o600)
		if err != nil {
			return fmt.Errorf("write isolated alternate checksums: %w", err)
		}
	}

	return nil
}
