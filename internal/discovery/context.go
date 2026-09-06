package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

type buildEnvironment struct {
	GOOS      string
	GOARCH    string
	GOVERSION string
	GOWORK    string
	GOFLAGS   string
}

func prepare(ctx context.Context, opts *Options) ([]string, []string, error) {
	opts.applyDefaults()
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve project root: %w", err)
	}
	opts.Root = root
	env := append(os.Environ(), opts.Env...)
	if opts.GOOS != "" {
		env = append(env, "GOOS="+opts.GOOS)
	}
	if opts.GOARCH != "" {
		env = append(env, "GOARCH="+opts.GOARCH)
	}
	if opts.Offline {
		env = append(env, "GOPROXY=off", "GONOPROXY=none", "GOSUMDB=off", "GOTOOLCHAIN=local")
	}
	command := exec.CommandContext(ctx, "go", "env", "-json", "GOOS", "GOARCH", "GOVERSION", "GOWORK", "GOFLAGS")
	command.Dir = root
	command.Env = env
	output, err := command.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("read Go build environment: %w", err)
	}
	var build buildEnvironment
	if err := json.Unmarshal(output, &build); err != nil {
		return nil, nil, fmt.Errorf("decode Go build environment: %w", err)
	}
	opts.GOOS, opts.GOARCH, opts.goVersion = build.GOOS, build.GOARCH, build.GOVERSION
	vendorRoot := root
	if build.GOWORK != "" && build.GOWORK != "off" {
		vendorRoot = filepath.Dir(build.GOWORK)
	}
	mode := "-mod=readonly"
	_, vendorErr := os.Stat(filepath.Join(vendorRoot, "vendor", "modules.txt"))
	if strings.Contains(build.GOFLAGS, "-mod=vendor") || (vendorErr == nil && !strings.Contains(build.GOFLAGS, "-mod=readonly")) {
		mode = "-mod=vendor"
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); os.IsNotExist(err) && filepath.Dir(build.GOWORK) == root && len(opts.Patterns) == 1 && opts.Patterns[0] == "./..." {
		data, err := os.ReadFile(build.GOWORK)
		if err != nil {
			return nil, nil, fmt.Errorf("read workspace: %w", err)
		}
		work, err := modfile.ParseWork(build.GOWORK, data, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("parse workspace: %w", err)
		}
		opts.Patterns = nil
		for _, use := range work.Use {
			module := use.Path
			if !filepath.IsAbs(module) {
				module = filepath.Join(root, module)
			}
			opts.Patterns = append(opts.Patterns, filepath.ToSlash(filepath.Join(module, "...")))
		}
	}
	return env, append([]string{mode}, buildFlags(opts.BuildTags)...), nil
}
