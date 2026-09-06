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

func GraphDigest(code *model.CodeModel) (string, error) {
	if code == nil || len(code.Modules) == 0 {
		return "", fmt.Errorf("module metadata is required")
	}
	snapshot := struct {
		GOOS            string        `json:"goos"`
		GOARCH          string        `json:"goarch"`
		Tags            []string      `json:"tags"`
		Modules         []graphModule `json:"modules"`
		Workspace       []string      `json:"workspace,omitempty"`
		WorkspaceSums   []string      `json:"workspaceSums,omitempty"`
		WorkspaceVendor []string      `json:"workspaceVendor,omitempty"`
	}{GOOS: code.GOOS, GOARCH: code.GOARCH, Tags: append([]string{}, code.BuildTags...)}
	sort.Strings(snapshot.Tags)
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
			manifest, err := moduleManifest(filepath.Join(dir, "go.mod"))
			if err != nil {
				return "", err
			}
			entry.Manifest = manifest
			entry.Sums, err = optionalLines(filepath.Join(dir, "go.sum"))
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
