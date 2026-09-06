package lockfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/modfile"
)

type graphModule struct {
	Path               string   `json:"path"`
	Version            string   `json:"version"`
	Main               bool     `json:"main"`
	Replacement        string   `json:"replacement,omitempty"`
	ReplacementVersion string   `json:"replacementVersion,omitempty"`
	Manifest           []string `json:"manifest,omitempty"`
	Sums               []string `json:"sums,omitempty"`
	Vendor             []string `json:"vendor,omitempty"`
}

type graphBuildEnvironment struct {
	GoVersion     string   `json:"goVersion"`
	GOOS          string   `json:"goos"`
	GOARCH        string   `json:"goarch"`
	BuildTags     []string `json:"buildTags,omitempty"`
	ModuleMode    string   `json:"moduleMode,omitempty"`
	Workspace     bool     `json:"workspace"`
	CGOEnabled    string   `json:"cgoEnabled,omitempty"`
	GOEXPERIMENT  string   `json:"goexperiment,omitempty"`
	GOFIPS140     string   `json:"gofips140,omitempty"`
	GOAMD64       string   `json:"goamd64,omitempty"`
	GOARM         string   `json:"goarm,omitempty"`
	GOARM64       string   `json:"goarm64,omitempty"`
	GO386         string   `json:"go386,omitempty"`
	GOMIPS        string   `json:"gomips,omitempty"`
	GOMIPS64      string   `json:"gomips64,omitempty"`
	GOPPC64       string   `json:"goppc64,omitempty"`
	GORISCV64     string   `json:"goriscv64,omitempty"`
	GOWASM        string   `json:"gowasm,omitempty"`
	CGOCFLAGS     string   `json:"cgoCFlags,omitempty"`
	CGOCPPFLAGS   string   `json:"cgoCPPFlags,omitempty"`
	CGOLDFLAGS    string   `json:"cgoLDFlags,omitempty"`
	CGOFFLAGS     string   `json:"cgoFFlags,omitempty"`
	CC            string   `json:"cc,omitempty"`
	CXX           string   `json:"cxx,omitempty"`
	CGOCXXFLAGS   string   `json:"cgoCXXFlags,omitempty"`
	SemanticFlags []string `json:"semanticFlags,omitempty"`
	ModFile       []string `json:"modFile,omitempty"`
	ModFileSums   []string `json:"modFileSums,omitempty"`
}

func GraphDigest(code *model.CodeModel) (string, error) {
	if code == nil || len(code.Modules) == 0 {
		return "", fmt.Errorf("module metadata is required")
	}
	snapshot := struct {
		GOOS            string                `json:"goos"`
		GOARCH          string                `json:"goarch"`
		Tags            []string              `json:"tags"`
		Modules         []graphModule         `json:"modules"`
		Workspace       []string              `json:"workspace,omitempty"`
		WorkspaceSums   []string              `json:"workspaceSums,omitempty"`
		WorkspaceVendor []string              `json:"workspaceVendor,omitempty"`
		EffectiveBuild  graphBuildEnvironment `json:"effectiveBuild"`
	}{GOOS: code.GOOS, GOARCH: code.GOARCH, Tags: append([]string{}, code.BuildTags...)}
	build := code.EffectiveBuild
	if build.GoVersion == "" {
		build.GoVersion = code.GoVersion
	}
	if build.GOOS == "" {
		build.GOOS = code.GOOS
	}
	if build.GOARCH == "" {
		build.GOARCH = code.GOARCH
	}
	if len(build.BuildTags) == 0 {
		build.BuildTags = append([]string(nil), code.BuildTags...)
	}
	if build.Workspace || code.WorkspaceFile != "" {
		build.Workspace = true
	}
	snapshot.EffectiveBuild = graphBuildEnvironment{
		GoVersion: build.GoVersion, GOOS: build.GOOS, GOARCH: build.GOARCH,
		BuildTags: append([]string(nil), build.BuildTags...), ModuleMode: build.ModuleMode,
		Workspace: build.Workspace, CGOEnabled: build.CGOEnabled, GOEXPERIMENT: build.GOEXPERIMENT,
		GOFIPS140: build.GOFIPS140, GOAMD64: build.GOAMD64, GOARM: build.GOARM,
		GO386: build.GO386, GOARM64: build.GOARM64, GOMIPS: build.GOMIPS, GOMIPS64: build.GOMIPS64,
		GOPPC64: build.GOPPC64, GORISCV64: build.GORISCV64, GOWASM: build.GOWASM,
		CGOCFLAGS: build.CGOCFLAGS, CGOCPPFLAGS: build.CGOCPPFLAGS,
		CGOLDFLAGS: build.CGOLDFLAGS, CGOFFLAGS: build.CGOFFLAGS,
		SemanticFlags: append([]string(nil), build.SemanticFlags...),
		CC:            build.CC, CXX: build.CXX, CGOCXXFLAGS: build.CGOCXXFLAGS,
	}
	effective := &snapshot.EffectiveBuild
	if build.GOARCH != "amd64" {
		effective.GOAMD64 = ""
	}
	if build.GOARCH != "arm" {
		effective.GOARM = ""
	}
	if build.GOARCH != "arm64" {
		effective.GOARM64 = ""
	}
	if build.GOARCH != "386" {
		effective.GO386 = ""
	}
	if build.GOARCH != "mips" && build.GOARCH != "mipsle" {
		effective.GOMIPS = ""
	}
	if build.GOARCH != "mips64" && build.GOARCH != "mips64le" {
		effective.GOMIPS64 = ""
	}
	if build.GOARCH != "ppc64" && build.GOARCH != "ppc64le" {
		effective.GOPPC64 = ""
	}
	if build.GOARCH != "riscv64" {
		effective.GORISCV64 = ""
	}
	if build.GOARCH != "wasm" {
		effective.GOWASM = ""
	}
	for _, value := range []*string{&effective.CC, &effective.CXX, &effective.CGOCFLAGS, &effective.CGOCPPFLAGS, &effective.CGOCXXFLAGS, &effective.CGOLDFLAGS, &effective.CGOFFLAGS} {
		if build.CGOEnabled == "0" {
			*value = ""
			continue
		}
		if code.ModuleRoot != "" {
			*value = strings.ReplaceAll(*value, filepath.Clean(code.ModuleRoot), "${PROJECT}")
		}
	}
	sort.Strings(snapshot.Tags)
	sort.Strings(snapshot.EffectiveBuild.BuildTags)
	sort.Strings(snapshot.EffectiveBuild.SemanticFlags)
	if build.ModFile != "" {
		manifest, err := moduleManifest(build.ModFile)
		if err != nil {
			return "", err
		}
		snapshot.EffectiveBuild.ModFile = manifest
		snapshot.EffectiveBuild.ModFileSums, err = optionalLines(companionSum(build.ModFile))
		if err != nil {
			return "", err
		}
	}
	for _, module := range code.Modules {
		entry := graphModule{Path: module.Path, Version: module.Version, Main: module.Main}
		local := module.Main
		dir := module.Dir
		if module.Replace != nil {
			entry.Replacement = module.Replace.Path
			entry.ReplacementVersion = module.Replace.Version
			if module.Replace.Version == "" {
				local = true
				dir = module.Replace.Dir
				entry.Replacement = "local"
			}
		}
		if local {
			if dir == "" {
				return "", fmt.Errorf("local module directory is unavailable")
			}
			manifestPath := filepath.Join(dir, "go.mod")
			if module.Main && code.EffectiveBuild.ModFile != "" {
				manifestPath = code.EffectiveBuild.ModFile
			}
			manifest, err := moduleManifest(manifestPath)
			if err != nil {
				return "", err
			}
			entry.Manifest = manifest
			entry.Sums, err = optionalLines(companionSum(manifestPath))
			if err != nil {
				return "", err
			}
			entry.Vendor, err = optionalLines(filepath.Join(dir, "vendor", "modules.txt"))
			if err != nil {
				return "", err
			}
		}
		snapshot.Modules = append(snapshot.Modules, entry)
	}
	sort.Slice(snapshot.Modules, func(i, j int) bool { return snapshot.Modules[i].Path < snapshot.Modules[j].Path })
	if code.WorkspaceFile != "" {
		data, err := os.ReadFile(code.WorkspaceFile)
		if err != nil {
			return "", fmt.Errorf("read workspace manifest: %w", err)
		}
		work, err := modfile.ParseWork(code.WorkspaceFile, data, nil)
		if err != nil {
			return "", fmt.Errorf("parse workspace manifest: %w", err)
		}
		for _, use := range work.Use {
			dir := use.Path
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(filepath.Dir(code.WorkspaceFile), dir)
			}
			data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
			if err != nil {
				return "", fmt.Errorf("read workspace module: %w", err)
			}
			module, err := modfile.Parse("go.mod", data, nil)
			if err != nil || module.Module == nil {
				return "", fmt.Errorf("workspace module has invalid manifest")
			}
			tokens := use.Syntax.Token
			tokens[len(tokens)-1] = module.Module.Mod.Path
		}
		normalizeReplacements(work.Replace)
		snapshot.Workspace = manifestLines(work.Syntax)
		snapshot.WorkspaceSums, err = optionalLines(code.WorkspaceFile + ".sum")
		if err != nil {
			return "", err
		}
		snapshot.WorkspaceVendor, err = optionalLines(filepath.Join(filepath.Dir(code.WorkspaceFile), "vendor", "modules.txt"))
		if err != nil {
			return "", err
		}
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", fmt.Errorf("encode module graph: %w", err)
	}
	return Digest(data), nil
}

func companionSum(modfile string) string {
	if strings.HasSuffix(modfile, ".mod") {
		return strings.TrimSuffix(modfile, ".mod") + ".sum"
	}
	return modfile + ".sum"
}

func moduleManifest(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read module manifest: %w", err)
	}
	parsed, err := modfile.Parse(filename, data, nil)
	if err != nil {
		return nil, fmt.Errorf("parse module manifest: %w", err)
	}
	normalizeReplacements(parsed.Replace)
	return manifestLines(parsed.Syntax), nil
}

func normalizeReplacements(replacements []*modfile.Replace) {
	for _, replacement := range replacements {
		if replacement.New.Version != "" {
			continue
		}
		for i, token := range replacement.Syntax.Token {
			if token == "=>" && i+1 < len(replacement.Syntax.Token) {
				replacement.Syntax.Token[i+1] = "local"
				break
			}
		}
	}
}

func manifestLines(syntax *modfile.FileSyntax) []string {
	var lines []string
	for _, statement := range syntax.Stmt {
		switch node := statement.(type) {
		case *modfile.Line:
			lines = append(lines, strings.Join(node.Token, " "))
		case *modfile.LineBlock:
			for _, line := range node.Line {
				lines = append(lines, strings.Join(node.Token, " ")+" "+strings.Join(line.Token, " "))
			}
		}
	}
	sort.Strings(lines)
	return lines
}

func optionalLines(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read module graph input: %w", err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		for i, field := range fields {
			if field == "=>" && i+2 == len(fields) {
				fields[i+1] = "local"
			}
		}
		if len(fields) > 0 {
			lines = append(lines, strings.Join(fields, " "))
		}
	}
	sort.Strings(lines)
	return lines, nil
}
