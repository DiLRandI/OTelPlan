package otelc

import (
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"gopkg.in/yaml.v3"
)

type AccessorFile struct {
	Name   string
	Source []byte
}

type accessorAdvice struct {
	File string `yaml:"file"`
	Path string `yaml:"path"`
}

type accessorAction struct {
	AddFile accessorAdvice `yaml:"add_file"`
}

type accessorRule struct {
	Target  string           `yaml:"target"`
	Actions []accessorAction `yaml:"do"`
}

// RenderAccessorRules returns injection rules and compile-only helper files for
// an importable provider package. It does not write files or load the provider.
func RenderAccessorRules(version string, code *model.CodeModel, plan model.ResolvedPlan, provider string) ([]byte, []AccessorFile, error) {
	if _, _, err := RenderRules(version, code, plan, provider); err != nil {
		return nil, nil, err
	}
	targets := append([]model.ResolvedTarget(nil), plan.Targets...)
	sort.Slice(targets, func(i, j int) bool { return targets[i].SymbolID < targets[j].SymbolID })
	document := yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	files := make([]AccessorFile, 0, len(targets))
	for _, target := range targets {
		if len(target.Attributes) == 0 {
			continue
		}
		source, _, err := RenderAccessors(code, target)
		if err != nil {
			return nil, nil, err
		}
		sum := sha256.Sum256([]byte(target.SymbolID))
		suffix := fmt.Sprintf("%x", sum[:8])
		name := "accessor_" + suffix + ".go"
		symbol, _ := code.Symbol(target.SymbolID)
		rule := accessorRule{Target: symbol.PackageImportPath, Actions: []accessorAction{{AddFile: accessorAdvice{File: name, Path: provider}}}}
		var node yaml.Node
		if err := node.Encode(rule); err != nil {
			return nil, nil, fmt.Errorf("encode accessor rule: %w", err)
		}
		document.Content = append(document.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "otelplan_accessor_" + suffix}, &node)
		files = append(files, AccessorFile{Name: name, Source: source})
	}
	data, err := yaml.Marshal(&document)
	if err != nil {
		return nil, nil, fmt.Errorf("encode accessor rules: %w", err)
	}
	return data, files, nil
}
