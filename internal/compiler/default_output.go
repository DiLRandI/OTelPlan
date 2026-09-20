package compiler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/mod/module"
)

func defaultBuildOutput(ctx context.Context, dir string, env, flags, targets []string, goos string) (string, error) {
	args := append([]string{"list", "-mod=readonly", "-json"}, flags...)
	args = append(args, targets...)
	command := exec.CommandContext(ctx, "go", args...)
	command.Dir, command.Env = dir, env

	data, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read build targets: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	count, name := 0, ""

	for {
		var pkg struct {
			Name, ImportPath  string
			GoFiles, CgoFiles []string
		}
		err := decoder.Decode(&pkg)

		if err == io.EOF {
			break
		} else if err != nil {
			return "", fmt.Errorf("decode build targets: %w", err)
		}

		count++

		if pkg.Name != "main" {
			continue
		}

		name = path.Base(pkg.ImportPath)

		if pkg.ImportPath == "command-line-arguments" {
			files := append(pkg.GoFiles, pkg.CgoFiles...)
			if len(files) == 0 {
				return "", errors.New("command has no source files")
			}

			name = strings.TrimSuffix(filepath.Base(files[0]), ".go")
		} else if prefix, suffix, ok := module.SplitPathVersion(pkg.ImportPath); ok && strings.HasPrefix(suffix, "/v") {
			name = path.Base(prefix)
		}
	}

	if count != 1 || name == "" {
		return "", nil
	}

	if goos == "windows" {
		name += ".exe"
	}

	return name, nil
}
