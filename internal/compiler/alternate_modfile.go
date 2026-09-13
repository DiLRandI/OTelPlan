package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func installAlternateModuleFiles(alternate, copiedDir string) error {
	manifest, err := os.ReadFile(alternate)
	if err != nil {
		return fmt.Errorf("read alternate module manifest")
	}
	sums, err := os.ReadFile(strings.TrimSuffix(alternate, ".mod") + ".sum")
	missingSums := os.IsNotExist(err)
	if err != nil && !missingSums {
		return fmt.Errorf("read alternate module checksums")
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		if err := os.Remove(filepath.Join(copiedDir, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove copied module file: %w", err)
		}
	}
	if err := os.WriteFile(filepath.Join(copiedDir, "go.mod"), manifest, 0600); err != nil {
		return fmt.Errorf("write isolated alternate manifest: %w", err)
	}
	if !missingSums {
		if err := os.WriteFile(filepath.Join(copiedDir, "go.sum"), sums, 0600); err != nil {
			return fmt.Errorf("write isolated alternate checksums: %w", err)
		}
	}
	return nil
}
