package lockfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/semver"
)

func Digest(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }

func Create(p *model.Policy, code *model.CodeModel, plan model.ResolvedPlan, backend model.LockBackend, moduleGraphDigest string, artifacts []model.ArtifactFile) (model.Lockfile, error) {
	var lock model.Lockfile
	if p == nil || code == nil || code.GoVersion == "" {
		return lock, fmt.Errorf("policy and analyzed Go version are required")
	}
	if backend.Name != p.Backend.Name || backend.Version != p.Backend.Version || !pinned(backend.Version) {
		return lock, fmt.Errorf("backend identity must match an exact pinned policy version")
	}
	if !validDigest(moduleGraphDigest) {
		return lock, fmt.Errorf("module graph digest is required")
	}
	policyData, err := json.Marshal(canonicalPolicy(*p))
	if err != nil {
		return lock, fmt.Errorf("encode policy: %w", err)
	}
	lock = model.Lockfile{APIVersion: model.LockAPIVersionV1Alpha1, PolicyDigest: Digest(policyData), GoVersion: code.GoVersion, ModuleGraphDigest: moduleGraphDigest, Backend: backend, Targets: []model.LockTarget{}, Artifacts: append([]model.ArtifactFile(nil), artifacts...)}
	seen := map[model.SymbolID]bool{}
	for _, target := range plan.Targets {
		if seen[target.SymbolID] {
			return model.Lockfile{}, fmt.Errorf("duplicate lock target")
		}
		seen[target.SymbolID] = true
		symbol, ok := code.Symbol(target.SymbolID)
		if !ok || symbol.Signature != target.Signature {
			return model.Lockfile{}, fmt.Errorf("target signature does not match analyzed symbol")
		}
		location := symbol.Location
		location.File = filepath.ToSlash(location.File)
		if !relative(location.File) {
			return model.Lockfile{}, fmt.Errorf("source path must be module-relative")
		}
		locked := model.LockTarget{Symbol: target.SymbolID, Signature: target.Signature, SignatureDigest: Digest([]byte(target.Signature)), SourceRule: target.RuleID, SpanName: target.SpanName, Context: model.LockContext{Strategy: target.ContextStrategy.Strategy, Index: target.ContextStrategy.Index}, Errors: model.LockErrors{Record: target.ErrorStrategy.Record, Indexes: append([]int(nil), target.ErrorStrategy.Indexes...)}, Location: location}
		for _, attr := range target.Attributes {
			access, err := validate.AttributeAccessor(code, *symbol, attr.From)
			if err != nil {
				return model.Lockfile{}, fmt.Errorf("resolve locked attribute: %w", err)
			}
			locked.Attributes = append(locked.Attributes, model.LockAttribute{Key: attr.Key, From: attr.From, Kind: access.Kind, Classification: attr.Classification, Allow: attr.Allow})
		}
		sort.Slice(locked.Attributes, func(i, j int) bool { return locked.Attributes[i].Key < locked.Attributes[j].Key })
		lock.Targets = append(lock.Targets, locked)
	}
	sort.Slice(lock.Targets, func(i, j int) bool { return lock.Targets[i].Symbol < lock.Targets[j].Symbol })
	sort.Slice(lock.Artifacts, func(i, j int) bool { return lock.Artifacts[i].Path < lock.Artifacts[j].Path })
	if err := check(lock); err != nil {
		return model.Lockfile{}, err
	}
	return lock, nil
}

func Marshal(lock model.Lockfile) ([]byte, error) {
	lock = canonical(lock)
	if err := check(lock); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode lockfile: %w", err)
	}
	return append(data, '\n'), nil
}

func Parse(data []byte) (model.Lockfile, error) {
	var lock model.Lockfile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&lock); err != nil {
		return lock, fmt.Errorf("decode lockfile: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return lock, fmt.Errorf("lockfile must contain one JSON document")
	}
	if err := check(lock); err != nil {
		return lock, err
	}
	return lock, nil
}

func Write(path string, lock model.Lockfile) error {
	data, err := Marshal(lock)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".otelplan-lock-*")
	if err != nil {
		return fmt.Errorf("create lockfile: %w", err)
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write lockfile: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close lockfile: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace lockfile: %w", err)
	}
	return nil
}

func check(lock model.Lockfile) error {
	if lock.APIVersion != model.LockAPIVersionV1Alpha1 || !validDigest(lock.PolicyDigest) || !validDigest(lock.ModuleGraphDigest) || lock.GoVersion == "" {
		return fmt.Errorf("invalid lockfile identity")
	}
	if lock.Backend.Name == "" || !pinned(lock.Backend.Version) {
		return fmt.Errorf("invalid locked backend")
	}
	if lock.Backend.Digest != "" && !validDigest(lock.Backend.Digest) {
		return fmt.Errorf("invalid backend digest")
	}
	seen := map[model.SymbolID]bool{}
	for _, target := range lock.Targets {
		if !relative(target.Location.File) {
			return fmt.Errorf("source path must be module-relative")
		}
		if target.Context.Strategy != model.ContextStrategyArgument && target.Context.Strategy != model.ContextStrategyRoot {
			return fmt.Errorf("invalid context strategy")
		}
		if target.Context.Index < 0 {
			return fmt.Errorf("invalid context index")
		}
		for _, index := range target.Errors.Indexes {
			if index < 0 || !target.Errors.Record {
				return fmt.Errorf("invalid error index")
			}
		}
		keys := map[string]bool{}
		for _, attr := range target.Attributes {
			if attr.Key == "" || keys[attr.Key] {
				return fmt.Errorf("invalid attribute key")
			}
			keys[attr.Key] = true
			switch attr.Kind {
			case "bool", "integer", "float", "string":
			default:
				return fmt.Errorf("invalid attribute kind")
			}
		}

		if target.Symbol == "" || seen[target.Symbol] || target.SourceRule == "" || target.SpanName == "" || target.SignatureDigest != Digest([]byte(target.Signature)) {
			return fmt.Errorf("invalid or duplicate locked target")
		}
		seen[target.Symbol] = true
	}
	paths := map[string]bool{}
	for _, artifact := range lock.Artifacts {
		if !relative(artifact.Path) || paths[artifact.Path] || !validDigest(artifact.Digest) {
			return fmt.Errorf("invalid artifact manifest")
		}
		paths[artifact.Path] = true
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, char := range value[7:] {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return false
		}
	}
	return true
}

func pinned(version string) bool {
	return semver.IsValid(version) && strings.HasPrefix(version, semver.MajorMinor(version)+".")
}

func relative(name string) bool {
	return name != "" && name != "." && name != ".." && !path.IsAbs(name) && !strings.ContainsAny(name, "\\:") && !strings.HasPrefix(name, "../") && path.Clean(name) == name
}

func canonical(lock model.Lockfile) model.Lockfile {
	lock.Targets = append([]model.LockTarget{}, lock.Targets...)
	for i := range lock.Targets {
		lock.Targets[i].Attributes = append([]model.LockAttribute(nil), lock.Targets[i].Attributes...)
		sort.Slice(lock.Targets[i].Attributes, func(a, b int) bool { return lock.Targets[i].Attributes[a].Key < lock.Targets[i].Attributes[b].Key })
	}
	sort.Slice(lock.Targets, func(i, j int) bool { return lock.Targets[i].Symbol < lock.Targets[j].Symbol })
	lock.Artifacts = append([]model.ArtifactFile(nil), lock.Artifacts...)
	sort.Slice(lock.Artifacts, func(i, j int) bool { return lock.Artifacts[i].Path < lock.Artifacts[j].Path })
	return lock
}
