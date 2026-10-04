package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"golang.org/x/tools/go/packages"
)

const (
	analysisCacheVersion = 1
	cacheMissingFile     = "missing"
)

type cachePackageInput struct {
	ID      string            `json:"id"`
	Path    string            `json:"path"`
	Name    string            `json:"name"`
	Dir     string            `json:"dir"`
	Files   []string          `json:"files"`
	Imports []string          `json:"imports"`
	Module  *cacheModuleInput `json:"module"`
}

type cacheModuleInput struct {
	GoVersion string            `json:"goVersion"`
	Path      string            `json:"path"`
	Version   string            `json:"version"`
	Dir       string            `json:"dir"`
	Main      bool              `json:"main"`
	Replace   *cacheModuleInput `json:"replace"`
}

type cacheFileInput struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type analysisCacheInput struct {
	Version             int                 `json:"version"`
	Implementation      string              `json:"implementation"`
	Root                string              `json:"root"`
	Environment         string              `json:"environment"`
	Patterns            []string            `json:"patterns"`
	CallGraph           bool                `json:"callGraph"`
	Tests               bool                `json:"tests"`
	IncludeDependencies bool                `json:"includeDependencies"`
	Packages            []cachePackageInput `json:"packages"`
	Files               []cacheFileInput    `json:"files"`
}

func analysisCacheKey(ctx context.Context, opts Options, pkgs []*packages.Package) (string, error) {
	packageInputs, paths := cachePackageInputs(opts, pkgs)

	files, err := cacheFileInputs(ctx, paths)
	if err != nil {
		return "", err
	}

	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate analysis cache implementation: %w", err)
	}

	implementation, err := cacheFileDigest(ctx, executable)
	if err != nil {
		return "", err
	}

	build, err := json.Marshal(opts.effectiveBuild)
	if err != nil {
		return "", fmt.Errorf("encode cache build identity: %w", err)
	}

	input := analysisCacheInput{
		Version: analysisCacheVersion, Implementation: implementation, Root: opts.Root,
		Environment: cacheDigest(build), Patterns: append([]string(nil), opts.Patterns...), CallGraph: opts.CallGraph,
		Tests: opts.IncludeTests, IncludeDependencies: opts.IncludeDependencies,
		Packages: packageInputs, Files: files,
	}
	slices.Sort(input.Patterns)

	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode analysis cache inputs: %w", err)
	}

	return cacheDigest(encoded), nil
}

func cachePackageInputs(opts Options, pkgs []*packages.Package) ([]cachePackageInput, map[string]bool) {
	var inputs []cachePackageInput

	paths := map[string]bool{}

	packages.Visit(pkgs, func(pkg *packages.Package) bool {
		files := slices.Concat(pkg.GoFiles, pkg.CompiledGoFiles, pkg.OtherFiles, pkg.EmbedFiles)
		slices.Sort(files)
		files = slices.Compact(files)

		for _, file := range files {
			paths[file] = true
		}

		imports := make([]string, 0, len(pkg.Imports))
		for path := range pkg.Imports {
			imports = append(imports, path)
		}

		slices.Sort(imports)
		inputs = append(inputs, cachePackageInput{
			ID: pkg.ID, Path: pkg.PkgPath, Name: pkg.Name, Dir: pkg.Dir,
			Files: files, Imports: imports, Module: cacheModuleIdentity(pkg.Module),
		})
		cacheModulePaths(opts, pkg.Module, paths)

		return true
	}, nil)
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].ID < inputs[j].ID })

	if opts.workspaceFile != "" {
		paths[opts.workspaceFile], paths[opts.workspaceFile+".sum"] = true, true
		if opts.effectiveBuild.ModuleMode == vendorModuleMode {
			paths[filepath.Join(filepath.Dir(opts.workspaceFile), "vendor", "modules.txt")] = true
		}
	}

	return inputs, paths
}

func cacheModuleIdentity(module *packages.Module) *cacheModuleInput {
	if module == nil {
		return nil
	}

	return &cacheModuleInput{
		Path: module.Path, Version: module.Version, Dir: module.Dir, Main: module.Main, GoVersion: module.GoVersion,
		Replace: cacheModuleIdentity(module.Replace),
	}
}

func cacheModulePaths(opts Options, module *packages.Module, paths map[string]bool) {
	if module == nil {
		return
	}

	if module.Replace != nil {
		cacheModulePaths(opts, module.Replace, paths)
	}

	manifest := module.GoMod
	if module.Main {
		manifest = filepath.Join(module.Dir, "go.mod")
		if opts.effectiveBuild.ModFile != "" && opts.workspaceFile == "" {
			manifest = opts.effectiveBuild.ModFile
		}
	}

	if module.Main && opts.effectiveBuild.ModuleMode == vendorModuleMode && opts.workspaceFile == "" {
		paths[filepath.Join(module.Dir, "vendor", "modules.txt")] = true
	}

	if manifest != "" {
		paths[manifest], paths[companionSum(manifest)] = true, true
	}
}

func cacheFileInputs(ctx context.Context, paths map[string]bool) ([]cacheFileInput, error) {
	names := make([]string, 0, len(paths))
	for path := range paths {
		names = append(names, path)
	}

	slices.Sort(names)
	inputs := make([]cacheFileInput, 0, len(names))

	for _, path := range names {
		digest, err := cacheFileDigest(ctx, path)
		if err != nil {
			return nil, err
		}

		inputs = append(inputs, cacheFileInput{Path: path, Digest: digest})
	}

	return inputs, nil
}

func cacheFileDigest(ctx context.Context, path string) (string, error) {
	err := ctx.Err()
	if err != nil {
		return "", fmt.Errorf("cache input hashing canceled: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(path)
	if os.IsNotExist(err) {
		return cacheMissingFile, nil
	}

	if err != nil {
		return "", fmt.Errorf("resolve cache input: %w", err)
	}

	directory, err := os.OpenRoot(filepath.Dir(resolved))
	if os.IsNotExist(err) {
		return cacheMissingFile, nil
	}

	if err != nil {
		return "", fmt.Errorf("open cache input directory: %w", err)
	}

	defer func() { _ = directory.Close() }()

	file, err := directory.Open(filepath.Base(resolved))
	if os.IsNotExist(err) {
		return cacheMissingFile, nil
	}

	if err != nil {
		return "", fmt.Errorf("read cache input: %w", err)
	}

	defer func() { _ = file.Close() }()

	hash := sha256.New()

	_, err = io.Copy(hash, file)
	if err != nil {
		return "", fmt.Errorf("hash cache input: %w", err)
	}

	err = ctx.Err()
	if err != nil {
		return "", fmt.Errorf("cache input hashing canceled: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func cacheDigest(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}
