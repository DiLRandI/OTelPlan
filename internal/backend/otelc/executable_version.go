package otelc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"golang.org/x/mod/semver"
)

const maximumVersionOutput = 4096

var errVersionOutputTooLarge = errors.New("otelc version output exceeds the size limit")

// ExecutableVersion queries an executable without asserting backend compatibility or identity.
// Only a valid version token is returned; build metadata and other output are omitted.
func ExecutableVersion(ctx context.Context, executable string) (string, error) {
	path, err := exec.LookPath(executable)
	if err != nil {
		return "", fmt.Errorf("find otelc executable: %w", err)
	}

	output, err := executableVersionOutput(ctx, path)
	if err != nil {
		return "", fmt.Errorf("query otelc version: %w", err)
	}

	fields := strings.Fields(string(output))
	if len(fields) < 3 || fields[0] != "otelc" || fields[1] != "version" || len(fields[2]) > 128 ||
		!semver.IsValid(fields[2]) {
		return "", errUnrecognizedVersion
	}

	version, _, _ := strings.Cut(fields[2], "+")
	if semver.Canonical(fields[2]) != version {
		return "", errUnrecognizedVersion
	}

	return version, nil
}

type versionOutput struct {
	buffer    bytes.Buffer
	oversized bool
}

func (output *versionOutput) Write(data []byte) (int, error) {
	size := len(data)

	remaining := maximumVersionOutput - output.buffer.Len()
	if size > remaining {
		output.oversized = true
		data = data[:remaining]
	}

	_, err := output.buffer.Write(data)
	if err != nil {
		return 0, fmt.Errorf("capture otelc version: %w", err)
	}

	return size, nil
}
