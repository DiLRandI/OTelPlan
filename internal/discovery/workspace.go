package discovery

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

func isolateWorkspace(filename string) (string, func(), error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", nil, fmt.Errorf("read workspace: %w", err)
	}
	work, err := modfile.ParseWork(filename, data, nil)
	if err != nil {
		return "", nil, fmt.Errorf("parse workspace: %w", err)
	}
	absolute := func(name string) string {
		if filepath.IsAbs(name) {
			return name
		}
		return filepath.Join(filepath.Dir(filename), name)
	}
	uses := make([]*modfile.Use, 0, len(work.Use))
	for _, use := range work.Use {
		uses = append(uses, &modfile.Use{Path: absolute(use.Path), ModulePath: use.ModulePath})
	}
	work.SetUse(uses)
	for _, replacement := range work.Replace {
		if replacement.New.Version == "" {
			if err := work.AddReplace(replacement.Old.Path, replacement.Old.Version, absolute(replacement.New.Path), ""); err != nil {
				return "", nil, err
			}
		}
	}
	work.Cleanup()
	dir, err := os.MkdirTemp("", "otelplan-workspace-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	fail := func(err error) (string, func(), error) { cleanup(); return "", nil, err }
	target := filepath.Join(dir, "go.work")
	if err := os.WriteFile(target, modfile.Format(work.Syntax), 0600); err != nil {
		return fail(err)
	}
	sums, err := os.ReadFile(filename + ".sum")
	if err == nil {
		if err := os.WriteFile(target+".sum", sums, 0600); err != nil {
			return fail(err)
		}
	} else if !os.IsNotExist(err) {
		return fail(err)
	}
	vendor := filepath.Join(filepath.Dir(filename), "vendor")
	if _, err := os.Stat(vendor); err == nil {
		if err := os.Symlink(vendor, filepath.Join(dir, "vendor")); err != nil {
			return fail(fmt.Errorf("isolate workspace vendor directory: %w", err))
		}
	} else if !os.IsNotExist(err) {
		return fail(err)
	}
	return target, cleanup, nil
}
