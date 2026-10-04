package compiler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	sourceCopyDirectoryMode = 0o700
	sourceCopyFileMode      = 0o600
	sourceCopyOwnerExecute  = 0o100
)

var (
	errNestedSourceStaging  = errors.New("source staging parent must be outside source tree")
	errExternalSourceLink   = errors.New("source link points outside copied tree")
	errNonregularSourceFile = errors.New("source tree contains a non-regular file")
)

type sourceTreeCopy struct {
	input  *os.Root
	output *os.Root
	source string
	target string
}

// CopySourceTree creates a disposable source copy. Internal symbolic links are
// relocated into the copy; external links require a wider source root.
func CopySourceTree(ctx context.Context, source, parent string) (string, error) {
	source, parent, err := sourceCopyPaths(source, parent)
	if err != nil {
		return "", err
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

	output, err := os.OpenRoot(target)
	if err != nil {
		return "", fmt.Errorf("open source copy root: %w", err)
	}

	defer func() { _ = output.Close() }()

	copier := sourceTreeCopy{input: input, output: output, source: source, target: target}

	err = copier.walk(ctx)
	if err != nil {
		return "", fmt.Errorf("copy source tree: %w", err)
	}

	completed = true

	return target, nil
}

func sourceCopyPaths(source, parent string) (string, string, error) {
	source, err := filepath.EvalSymlinks(source)
	if err != nil {
		return "", "", fmt.Errorf("resolve source directory: %w", err)
	}

	source, err = filepath.Abs(source)
	if err != nil {
		return "", "", fmt.Errorf("resolve absolute source directory: %w", err)
	}

	if parent == "" {
		parent = os.TempDir()
	}

	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return "", "", fmt.Errorf("resolve staging parent: %w", err)
	}

	parent, err = filepath.Abs(parent)
	if err != nil {
		return "", "", fmt.Errorf("resolve absolute staging parent: %w", err)
	}

	if withinTree(source, parent) {
		return "", "", errNestedSourceStaging
	}

	return source, parent, nil
}

func (tree sourceTreeCopy) walk(ctx context.Context) error {
	err := fs.WalkDir(tree.input.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("read source entry: %w", walkErr)
		}

		err := ctx.Err()
		if err != nil {
			return fmt.Errorf("source copy canceled: %w", err)
		}

		if path == "." {
			return nil
		}

		return tree.entry(filepath.FromSlash(path), entry)
	})
	if err != nil {
		return fmt.Errorf("walk source entries: %w", err)
	}

	return nil
}

func (tree sourceTreeCopy) entry(path string, entry fs.DirEntry) error {
	if entry.IsDir() {
		err := tree.output.Mkdir(path, sourceCopyDirectoryMode)
		if err != nil {
			return fmt.Errorf("create copied source directory: %w", err)
		}

		return nil
	}

	if entry.Type()&os.ModeSymlink != 0 {
		return tree.link(path)
	}

	if !entry.Type().IsRegular() {
		return errNonregularSourceFile
	}

	return tree.file(path)
}

func (tree sourceTreeCopy) link(path string) error {
	resolved, err := filepath.EvalSymlinks(filepath.Join(tree.source, path))
	if err != nil {
		return fmt.Errorf("resolve source link: %w", err)
	}

	if !withinTree(tree.source, resolved) {
		return errExternalSourceLink
	}

	relative, err := filepath.Rel(tree.source, resolved)
	if err != nil {
		return fmt.Errorf("locate source link target: %w", err)
	}

	destination := filepath.Join(tree.target, path)

	link, err := filepath.Rel(filepath.Dir(destination), filepath.Join(tree.target, relative))
	if err != nil {
		return fmt.Errorf("relocate source link target: %w", err)
	}

	err = tree.output.Symlink(link, path)
	if err != nil {
		return fmt.Errorf("create relocated source link: %w", err)
	}

	return nil
}

func (tree sourceTreeCopy) file(path string) error {
	input, err := tree.input.Open(path)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}

	defer func() { _ = input.Close() }()

	info, err := input.Stat()
	if err != nil {
		return fmt.Errorf("inspect source file: %w", err)
	}

	mode := sourceCopyFileMode | info.Mode().Perm()&sourceCopyOwnerExecute

	output, err := tree.output.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create copied source file: %w", err)
	}

	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()

	if copyErr != nil {
		return fmt.Errorf("copy source file contents: %w", copyErr)
	}

	if closeErr != nil {
		return fmt.Errorf("close copied source file: %w", closeErr)
	}

	return nil
}

func withinTree(root, path string) bool {
	relative, err := filepath.Rel(root, path)

	return err == nil && (relative == "." || filepath.IsLocal(relative))
}
