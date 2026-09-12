package otelc

import (
	_ "embed"
	"fmt"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

//go:embed runtime/go.mod.txt
var runtimeModule []byte

//go:embed runtime/go.sum.txt
var runtimeSums []byte

func renderRuntimeModule(modulePath string) ([]GeneratedFile, error) {
	if err := module.CheckPath(modulePath); err != nil {
		return nil, fmt.Errorf("invalid generated module path")
	}
	file, err := modfile.Parse("go.mod", runtimeModule, nil)
	if err != nil {
		return nil, fmt.Errorf("parse pinned runtime module: %w", err)
	}
	if err := file.AddModuleStmt(modulePath); err != nil {
		return nil, fmt.Errorf("set runtime module path: %w", err)
	}
	data, err := file.Format()
	if err != nil {
		return nil, fmt.Errorf("format runtime module: %w", err)
	}
	return []GeneratedFile{{Path: "go.mod", Data: data}, {Path: "go.sum", Data: append([]byte(nil), runtimeSums...)}}, nil
}
