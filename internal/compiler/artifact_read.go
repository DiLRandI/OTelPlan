package compiler

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"syscall"
)

var errArtifactFileChanged = errors.New("artifact file changed while opening")

func readRegularArtifact(root *os.Root, name string) ([]byte, error) {
	expected, err := root.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("inspect artifact file: %w", err)
	}

	if !expected.Mode().IsRegular() {
		return nil, errNonregularStagedArtifact
	}

	// A replacement after Lstat must not turn opening the file into a FIFO wait.
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open artifact file: %w", err)
	}

	data, readErr := readOpenedArtifact(file, expected)

	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(readErr, closeErr)
	}

	return data, nil
}

func readOpenedArtifact(file *os.File, expected fs.FileInfo) ([]byte, error) {
	opened, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect opened artifact file: %w", err)
	}

	if !opened.Mode().IsRegular() {
		return nil, errNonregularStagedArtifact
	}

	if !os.SameFile(expected, opened) {
		return nil, errArtifactFileChanged
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read artifact file: %w", err)
	}

	return data, nil
}
