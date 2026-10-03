package discovery

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const (
	maximumCacheEntryBytes = 64 << 20
	cacheDirectoryMode     = 0o700
	cacheFileMode          = 0o600
)

type cacheLookup struct {
	code *model.CodeModel
}

type analysisCacheEntry struct {
	Version int             `json:"version"`
	Key     string          `json:"key"`
	Digest  string          `json:"digest"`
	Model   json.RawMessage `json:"model"`
}

func openAnalysisCache(path string) (*os.Root, error) {
	err := os.MkdirAll(path, cacheDirectoryMode)
	if err != nil {
		return nil, fmt.Errorf("create requested analysis cache: %w", err)
	}

	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("open requested analysis cache: %w", err)
	}

	return root, nil
}

func readAnalysisCache(root *os.Root, key string, opts Options) (cacheLookup, error) {
	file, err := root.Open(key + ".json")
	if os.IsNotExist(err) {
		return cacheLookup{code: nil}, nil
	}

	if err != nil {
		return cacheLookup{code: nil}, fmt.Errorf("read analysis cache entry: %w", err)
	}

	defer func() { _ = file.Close() }()

	contents, err := io.ReadAll(io.LimitReader(file, maximumCacheEntryBytes+1))
	if err != nil {
		return cacheLookup{code: nil}, fmt.Errorf("read analysis cache payload: %w", err)
	}

	if len(contents) > maximumCacheEntryBytes {
		return cacheLookup{code: nil}, nil
	}

	return decodeAnalysisCache(contents, key, opts), nil
}

func decodeAnalysisCache(contents []byte, key string, opts Options) cacheLookup {
	var entry analysisCacheEntry

	err := json.Unmarshal(contents, &entry)
	if err != nil || entry.Version != analysisCacheVersion || entry.Key != key ||
		cacheDigest(entry.Model) != entry.Digest {
		return cacheLookup{code: nil}
	}

	code := new(model.CodeModel)

	err = json.Unmarshal(entry.Model, code)
	if err != nil || code.ModuleRoot != opts.Root || code.GoVersion != opts.goVersion {
		return cacheLookup{code: nil}
	}

	code.EffectiveBuild = opts.effectiveBuild

	return cacheLookup{code: code}
}

func writeAnalysisCache(root *os.Root, key string, code *model.CodeModel) error {
	snapshot := *code

	var emptyBuild model.BuildEnvironment

	snapshot.EffectiveBuild = emptyBuild

	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode analysis cache model: %w", err)
	}

	entry := analysisCacheEntry{Version: analysisCacheVersion, Key: key, Digest: cacheDigest(payload), Model: payload}

	contents, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode analysis cache entry: %w", err)
	}

	if len(contents) > maximumCacheEntryBytes {
		return nil
	}

	name := "temporary-" + rand.Text()

	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, cacheFileMode)
	if err != nil {
		return fmt.Errorf("stage analysis cache entry: %w", err)
	}
	defer func() { _ = root.Remove(name) }()

	_, err = file.Write(contents)
	if err != nil {
		_ = file.Close()

		return fmt.Errorf("write analysis cache entry: %w", err)
	}

	err = file.Close()
	if err != nil {
		return fmt.Errorf("close analysis cache entry: %w", err)
	}

	err = root.Rename(name, key+".json")
	if err != nil {
		return fmt.Errorf("publish analysis cache entry: %w", err)
	}

	return nil
}
