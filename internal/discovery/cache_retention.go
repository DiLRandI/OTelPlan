package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/version"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const (
	maximumAnalysisCacheEntries = 64
	maximumAnalysisCacheBytes   = 256 << 20
)

type cacheRetentionEntry struct {
	name     string
	size     int64
	modified time.Time
}

func pruneAnalysisCache(ctx context.Context, root *os.Root, keep string, maximumEntries int,
	maximumBytes int64) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("prune analysis cache canceled: %w", err)
	}

	entries, err := retainedAnalysisCacheEntries(ctx, root)
	if err != nil {
		return err
	}

	slices.SortFunc(entries, func(a, b cacheRetentionEntry) int {
		if order := a.modified.Compare(b.modified); order != 0 {
			return order
		}

		return strings.Compare(a.name, b.name)
	})

	count := len(entries)

	var total int64

	for _, entry := range entries {
		total += entry.size
	}

	for _, entry := range entries {
		if count <= maximumEntries && total <= maximumBytes {
			break
		}

		if entry.name == keep+".json" {
			continue
		}

		err = removeRetainedAnalysisCacheEntry(ctx, root, entry.name)
		if err != nil {
			return err
		}

		count--
		total -= entry.size
	}

	return nil
}

func removeRetainedAnalysisCacheEntry(ctx context.Context, root *os.Root, name string) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("prune analysis cache canceled: %w", err)
	}

	err = root.Remove(name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove expired analysis cache entry: %w", err)
	}

	return nil
}

func retainedAnalysisCacheEntries(ctx context.Context, root *os.Root) ([]cacheRetentionEntry, error) {
	files, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, fmt.Errorf("list analysis cache entries: %w", err)
	}

	entries := make([]cacheRetentionEntry, 0, len(files))

	for _, file := range files {
		err := ctx.Err()
		if err != nil {
			return nil, fmt.Errorf("inspect analysis cache canceled: %w", err)
		}

		key, ok := analysisCacheFileKey(file.Name())
		if !ok {
			continue
		}

		entry, owned, err := retainedAnalysisCacheEntry(root, file.Name(), key)
		if err != nil {
			return nil, err
		}

		if owned {
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

func retainedAnalysisCacheEntry(root *os.Root, name, key string) (cacheRetentionEntry, bool, error) {
	var empty cacheRetentionEntry

	info, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return empty, false, nil
	}

	if err != nil {
		return empty, false, fmt.Errorf("inspect retained analysis cache entry: %w", err)
	}

	if !info.Mode().IsRegular() || info.Size() > maximumCacheEntryBytes {
		return empty, false, nil
	}

	owned, err := ownsAnalysisCacheEntry(root, name, key)
	if err != nil || !owned {
		return empty, false, err
	}

	return cacheRetentionEntry{name: name, size: info.Size(), modified: info.ModTime()}, true, nil
}

func analysisCacheFileKey(name string) (string, bool) {
	key, ok := strings.CutSuffix(name, ".json")
	if !ok || len(key) != 64 {
		return "", false
	}

	for _, character := range key {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return "", false
		}
	}

	return key, true
}

func ownsAnalysisCacheEntry(root *os.Root, name, key string) (bool, error) {
	file, err := root.Open(name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("open retained analysis cache entry: %w", err)
	}

	defer func() { _ = file.Close() }()

	contents, err := io.ReadAll(io.LimitReader(file, maximumCacheEntryBytes+1))
	if err != nil {
		return false, fmt.Errorf("read retained analysis cache entry: %w", err)
	}

	if len(contents) > maximumCacheEntryBytes {
		return false, nil
	}

	return ownsAnalysisCachePayload(contents, key), nil
}

func ownsAnalysisCachePayload(contents []byte, key string) bool {
	var entry analysisCacheEntry

	err := json.Unmarshal(contents, &entry)
	if err != nil || entry.Version != analysisCacheVersion || entry.Key != key ||
		entry.Digest != cacheDigest(entry.Model) {
		return false
	}

	code := new(model.CodeModel)

	err = json.Unmarshal(entry.Model, code)

	return err == nil && code.ModuleRoot != "" && version.IsValid(code.GoVersion)
}
