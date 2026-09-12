package compiler

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// CopySourceTree creates a disposable source copy. Internal symbolic links are
// relocated into the copy; external links require a wider source root.
func CopySourceTree(ctx context.Context, source, parent string) (string, error) {
	source, err := filepath.EvalSymlinks(source)
	if err != nil {
		return "", fmt.Errorf("resolve source directory: %w", err)
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return "", err
	}
	if parent == "" {
		parent = os.TempDir()
	}
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return "", fmt.Errorf("resolve staging parent: %w", err)
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return "", err
	}
	if withinTree(source, parent) {
		return "", fmt.Errorf("source staging parent must be outside source tree")
	}
	input, err := os.OpenRoot(source)
	if err != nil {
		return "", fmt.Errorf("open source tree: %w", err)
	}
	defer func() { _ = input.Close() }()
	target, err := os.MkdirTemp(parent, "otelplan-source-")
	if err != nil {
		return "", fmt.Errorf("create source copy: %w", err)
	}
	completed := false
	defer func() {
		if !completed {
			_ = os.RemoveAll(target)
		}
	}()
	err = fs.WalkDir(input.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		destination := filepath.Join(target, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.Mkdir(destination, 0700)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(filepath.Join(source, filepath.FromSlash(path)))
			if err != nil {
				return fmt.Errorf("resolve source link: %w", err)
			}
			if !withinTree(source, resolved) {
				return fmt.Errorf("source link points outside copied tree")
			}
			relative, err := filepath.Rel(source, resolved)
			if err != nil {
				return err
			}
			link, err := filepath.Rel(filepath.Dir(destination), filepath.Join(target, relative))
			if err != nil {
				return err
			}
			return os.Symlink(link, destination)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("source tree contains a non-regular file")
		}
		file, err := input.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		info, err := file.Stat()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600|(info.Mode().Perm()&0100))
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, file)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		return "", fmt.Errorf("copy source tree: %w", err)
	}
	completed = true
	return target, nil
}

func withinTree(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && (relative == "." || filepath.IsLocal(relative))
}
