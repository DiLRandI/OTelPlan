package compiler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

var errEmptyBuildSelection = errors.New("build module selection is empty")

// ReadModuleSelection lists the modules selected in a prepared directory. Env
// must include the effective build environment and isolated workspace path.
func ReadModuleSelection(ctx context.Context, dir string, env []string) ([]model.ModuleInfo, error) {
	command := exec.CommandContext(ctx, "go", "list", "-mod=readonly", "-m", "-json", "all")
	command.Dir = dir
	command.Env = env

	output, err := command.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("read build module selection: %w", ctx.Err())
		}

		return nil, fmt.Errorf("read build module selection: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	modules := []model.ModuleInfo{}

	for {
		var module model.ModuleInfo

		err := decoder.Decode(&module)
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("decode build module selection: %w", err)
		}

		modules = append(modules, module)
	}

	if len(modules) == 0 {
		return nil, errEmptyBuildSelection
	}

	_, err = indexModules(modules)
	if err != nil {
		return nil, err
	}

	return modules, nil
}
