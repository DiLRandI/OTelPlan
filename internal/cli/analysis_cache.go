package cli

import (
	"errors"
	"path/filepath"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
)

func configureAnalysisCache(analysis *discovery.Options, opts options, command string) {
	if opts.cacheDir == "" {
		return
	}

	analysis.CacheDir = opts.cacheDir
	if !filepath.IsAbs(analysis.CacheDir) {
		analysis.CacheDir = filepath.Join(opts.root, analysis.CacheDir)
	}

	analysis.CacheReadOnly = opts.check || opts.dryRun || command == diffCommandName
}

func analysisFailureMessage(err error) string {
	if errors.Is(err, discovery.ErrCacheStorage) {
		return "cannot access requested analysis cache"
	}

	return err.Error()
}
