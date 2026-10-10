package discovery

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func readBuildMetadata(filename string) ([]byte, error) {
	// Caller-selected metadata may be symlinked outside the checkout. Anchor the
	// read to its resolved parent without changing the recorded input identity.
	resolved, err := filepath.EvalSymlinks(filename)
	if err != nil {
		return nil, fmt.Errorf("resolve build metadata: %w", err)
	}

	directory, err := os.OpenRoot(filepath.Dir(resolved))
	if err != nil {
		return nil, fmt.Errorf("open build metadata directory: %w", err)
	}

	contents, readErr := directory.ReadFile(filepath.Base(resolved))

	closeErr := directory.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close build metadata directory: %w", closeErr)
	}

	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("read build metadata: %w", errors.Join(readErr, closeErr))
	}

	return contents, nil
}

func isolateModuleManifest(original string) (string, func(), error) {
	manifest, err := readBuildMetadata(original)
	if err != nil {
		return "", nil, fmt.Errorf("read effective module manifest: %w", err)
	}

	directory, err := os.MkdirTemp("", "otelplan-effective-*")
	if err != nil {
		return "", nil, fmt.Errorf("create isolated module directory: %w", err)
	}

	cleanup := func() { _ = os.RemoveAll(directory) }

	root, err := os.OpenRoot(directory)
	if err != nil {
		cleanup()

		return "", nil, fmt.Errorf("open isolated module directory: %w", err)
	}

	writeErr := writeModuleMetadata(root, original, manifest)

	closeErr := root.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close isolated module directory: %w", closeErr)
	}

	if writeErr != nil || closeErr != nil {
		cleanup()

		return "", nil, errors.Join(writeErr, closeErr)
	}

	return filepath.Join(directory, "effective.mod"), cleanup, nil
}

func writeModuleMetadata(root *os.Root, original string, manifest []byte) error {
	err := root.WriteFile("effective.mod", manifest, isolatedManifestMode)
	if err != nil {
		return fmt.Errorf("write isolated module manifest: %w", err)
	}

	checksums, err := readBuildMetadata(companionSum(original))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("read effective module checksums: %w", err)
	}

	err = root.WriteFile("effective.sum", checksums, isolatedManifestMode)
	if err != nil {
		return fmt.Errorf("write isolated module checksums: %w", err)
	}

	return nil
}
