package cli

import (
	"errors"
	"path/filepath"
	"strings"

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

func analysisFailureMessage(err error, offline bool) string {
	if errors.Is(err, discovery.ErrCacheStorage) {
		return "cannot access requested analysis cache"
	}

	if errors.Is(err, discovery.ErrPackageAnalysis) {
		const offlineLookup = "module lookup disabled by GOPROXY=off"

		if offline && strings.Contains(err.Error(), offlineLookup) {
			return "Go package analysis failed: " + offlineLookup + "; prepare dependencies before using --offline"
		}

		return "Go package analysis failed; review the selected packages and Go build configuration"
	}

	return err.Error()
}
