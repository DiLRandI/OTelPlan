package compiler

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
)

func materializeVendorWorkspace(ctx context.Context, workspace PreparedWorkspace, originalVendor string, env []string) error {
	if _, err := os.Stat(filepath.Join(originalVendor, "modules.txt")); err != nil {
		return fmt.Errorf("read analyzed vendor manifest: %w", err)
	}

	command := exec.CommandContext(ctx, "go", "work", "vendor")
	command.Dir = workspace.Dir
	command.Env = append(append([]string(nil), env...), "GOWORK="+workspace.WorkspaceFile, "GOFLAGS=")

	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return fmt.Errorf("materialize isolated vendor workspace (dependencies must be available in the module cache): %w", err)
	}

	if err := compareVendorFiles(originalVendor, filepath.Join(workspace.Dir, "vendor")); err != nil {
		return fmt.Errorf("isolated vendor content differs from analyzed source: %w", err)
	}

	return nil
}

func compareVendorFiles(originalDir, generatedDir string) error {
	original, err := os.OpenRoot(originalDir)
	if err != nil {
		return fmt.Errorf("open analyzed vendor tree: %w", err)
	}
	defer func() { _ = original.Close() }()

	generated, err := os.OpenRoot(generatedDir)
	if err != nil {
		return fmt.Errorf("open isolated vendor tree: %w", err)
	}
	defer func() { _ = generated.Close() }()

	originalFiles := map[string]map[string]bool{}

	err = fs.WalkDir(original.FS(), ".", func(vendorPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() || vendorPath == "modules.txt" {
			return nil
		}

		if !entry.Type().IsRegular() {
			return fmt.Errorf("analyzed vendor file %s is not regular", vendorPath)
		}

		directory, name := path.Split(vendorPath)
		directory = path.Clean(directory)

		if originalFiles[directory] == nil {
			originalFiles[directory] = map[string]bool{}
		}

		originalFiles[directory][name] = true

		return compareVendorFile(original, generated, vendorPath)
	})
	if err != nil {
		return fmt.Errorf("compare vendor trees: %w", err)
	}
	if err := rejectAddedVendorFiles(generated, originalFiles); err != nil {
		return err
	}

	return nil
}

func rejectAddedVendorFiles(generated *os.Root, originals map[string]map[string]bool) error {
	directories := make([]string, 0, len(originals))
	for directory := range originals {
		directories = append(directories, directory)
	}

	sort.Strings(directories)

	for _, directory := range directories {
		entries, err := fs.ReadDir(generated.FS(), directory)
		if err != nil {
			return fmt.Errorf("read isolated vendor directory %s: %w", directory, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() && !originals[directory][entry.Name()] {
				return fmt.Errorf("vendored file %s was added", path.Join(directory, entry.Name()))
			}
		}
	}

	return nil
}

func compareVendorFile(original, generated *os.Root, path string) error {
	originalDigest, err := vendorFileDigest(original, path)
	if err != nil {
		return fmt.Errorf("read analyzed vendor file %s: %w", path, err)
	}

	generatedDigest, err := vendorFileDigest(generated, path)
	if err != nil {
		return fmt.Errorf("read isolated vendor file %s: %w", path, err)
	}

	if originalDigest != generatedDigest {
		return fmt.Errorf("vendored file %s changed", path)
	}

	return nil
}

func vendorFileDigest(root *os.Root, path string) ([sha256.Size]byte, error) {
	file, err := root.Open(path)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("open vendor file: %w", err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("stat vendor file: %w", err)
	}

	if !info.Mode().IsRegular() {
		return [sha256.Size]byte{}, errors.New("vendor entry is not a regular file")
	}

	hash := sha256.New()

	if _, err := io.Copy(hash, file); err != nil {
		return [sha256.Size]byte{}, err
	}

	return [sha256.Size]byte(hash.Sum(nil)), nil
}
