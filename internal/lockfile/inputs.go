package lockfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Graph inputs are caller-owned manifests/checksums, including replacements
// outside the checkout. Resolve legitimate symlinks before anchoring each read.
func readGraphInput(filename string) ([]byte, error) {
	resolved, err := filepath.EvalSymlinks(filename)
	if err != nil {
		return nil, fmt.Errorf("resolve graph input: %w", err)
	}

	directory, err := os.OpenRoot(filepath.Dir(resolved))
	if err != nil {
		return nil, fmt.Errorf("open graph input directory: %w", err)
	}

	defer func() { _ = directory.Close() }()

	contents, err := directory.ReadFile(filepath.Base(resolved))
	if err != nil {
		return nil, fmt.Errorf("read graph input: %w", err)
	}

	return contents, nil
}
