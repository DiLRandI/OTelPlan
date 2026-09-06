package discovery

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/tools/go/packages"
)

type Options struct {
	Root                string
	Patterns            []string
	BuildTags           []string
	IncludeTests        bool
	IncludeDependencies bool
	GOOS                string
	GOARCH              string
}

func (o *Options) applyDefaults() {
	if o.Root == "" {
		o.Root = "."
	}
	if len(o.Patterns) == 0 {
		o.Patterns = []string{"./..."}
	}
}

func Load(opts Options) (*model.CodeModel, error) {
	opts.applyDefaults()
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.GOARCH == "" {
		opts.GOARCH = runtime.GOARCH
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports |
			packages.NeedModule | packages.NeedDeps | packages.NeedCompiledGoFiles,
		Dir:        opts.Root,
		Tests:      opts.IncludeTests,
		BuildFlags: append([]string{"-mod=readonly"}, buildFlags(opts.BuildTags)...),
		Env:        os.Environ(),
	}
	if opts.GOOS != "" {
		cfg.Env = append(cfg.Env, "GOOS="+opts.GOOS)
	}
	if opts.GOARCH != "" {
		cfg.Env = append(cfg.Env, "GOARCH="+opts.GOARCH)
	}

	pkgs, err := packages.Load(cfg, opts.Patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	if err := reportErrors(pkgs); err != nil {
		return nil, err
	}

	var selected []*packages.Package
	seen := map[string]*packages.Package{}
	packages.Visit(pkgs, func(p *packages.Package) bool {
		if (opts.IncludeDependencies || (p.Module != nil && p.Module.Main)) && !strings.HasSuffix(p.PkgPath, ".test") {
			if previous := seen[p.PkgPath]; previous == nil || len(p.Syntax) > len(previous.Syntax) {
				seen[p.PkgPath] = p
			}
		}
		return true
	}, nil)
	for _, p := range seen {
		selected = append(selected, p)
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].PkgPath < selected[j].PkgPath })
	return buildModel(selected, opts), nil
}

func buildFlags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	return []string{"-tags=" + strings.Join(tags, ",")}
}

func reportErrors(pkgs []*packages.Package) error {
	var errs []string
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			errs = append(errs, e.Error())
		}
	})
	if len(errs) > 0 {
		return fmt.Errorf("package analysis failed: %s", strings.Join(errs, "; "))
	}
	return nil
}
