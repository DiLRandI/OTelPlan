package discovery

import (
	"context"
	"fmt"
	"os"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/tools/go/packages"
)

func loadCacheInputs(ctx context.Context, opts Options, cfg *packages.Config) (string, error) {
	metadata := *cfg
	metadata.Mode = packages.NeedName | packages.NeedFiles | packages.NeedModule | packages.NeedImports |
		packages.NeedDeps | packages.NeedCompiledGoFiles | packages.NeedEmbedFiles

	pkgs, err := packages.Load(&metadata, opts.Patterns...)
	if err != nil {
		return "", fmt.Errorf("load cache package metadata: %w", err)
	}

	err = reportErrors(pkgs)
	if err != nil {
		return "", err
	}

	return analysisCacheKey(ctx, opts, pkgs)
}

func loadCachedModel(ctx context.Context, opts Options, cfg *packages.Config) (*model.CodeModel, bool, error) {
	key, err := loadCacheInputs(ctx, opts, cfg)
	if err != nil {
		return nil, false, err
	}

	cache, err := openAnalysisCache(opts.CacheDir)
	if err != nil {
		return nil, false, err
	}

	defer func() { _ = cache.Close() }()

	lookup, err := readAnalysisCache(cache, key, opts)
	if err != nil {
		return nil, false, err
	}

	err = ctx.Err()
	if err != nil {
		return nil, false, fmt.Errorf("cached analysis canceled: %w", err)
	}

	if lookup.code != nil {
		return lookup.code, true, nil
	}

	code, err := refreshAnalysisCache(ctx, cache, key, opts, cfg)

	return code, false, err
}

func refreshAnalysisCache(ctx context.Context, cache *os.Root, key string, opts Options,
	cfg *packages.Config) (*model.CodeModel, error) {
	full := *cfg
	full.Mode |= packages.NeedEmbedFiles

	result, err := loadPreparedModel(ctx, opts, &full)
	if err != nil {
		return nil, err
	}

	after, err := analysisCacheKey(ctx, opts, result.packages)
	if err != nil {
		return nil, err
	}

	if key == after {
		err = writeAnalysisCache(cache, key, result.code)
		if err != nil {
			return nil, err
		}
	}

	return result.code, nil
}
