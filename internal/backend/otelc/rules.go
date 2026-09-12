package otelc

import (
	"crypto/sha256"
	"fmt"
	"go/token"
	"sort"
	"strconv"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/module"
	"gopkg.in/yaml.v3"
)

type HookBinding struct {
	Symbol model.SymbolID
	Before string
	After  string
}

type functionSelector struct {
	Function string `yaml:"func"`
	Receiver string `yaml:"recv,omitempty"`
}

type hookAdvice struct {
	Before string `yaml:"before"`
	After  string `yaml:"after"`
	Path   string `yaml:"path"`
}

type hookAction struct {
	Hooks hookAdvice `yaml:"inject_hooks"`
}

type functionRule struct {
	Target  string           `yaml:"target"`
	Where   functionSelector `yaml:"where"`
	Actions []hookAction     `yaml:"do"`
}

// RenderRules produces exact function-entry selectors and stable hook names.
// Hook source generation and executable verification are separate phases.
func RenderRules(version string, code *model.CodeModel, plan model.ResolvedPlan, hookImportPath string) ([]byte, []HookBinding, error) {
	if err := module.CheckImportPath(hookImportPath); err != nil {
		return nil, nil, fmt.Errorf("invalid generated hook import path")
	}
	if diagnostics := Check(version, code, plan); diagnostics.HasErrors() {
		return nil, nil, fmt.Errorf("plan is incompatible with the pinned backend: %s", diagnostics.Errors()[0].Message)
	}
	targets := append([]model.ResolvedTarget(nil), plan.Targets...)
	sort.Slice(targets, func(i, j int) bool { return targets[i].SymbolID < targets[j].SymbolID })
	document := yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	bindings := make([]HookBinding, 0, len(targets))
	seen := map[string]bool{}
	for _, target := range targets {
		symbol, _ := code.Symbol(target.SymbolID)
		parsed, err := model.ParseSymbolID(target.SymbolID)
		if err != nil || parsed.ImportPath != symbol.PackageImportPath || parsed.Name != symbol.Name {
			return nil, nil, fmt.Errorf("backend target has inconsistent canonical identity")
		}
		if module.CheckImportPath(symbol.PackageImportPath) != nil || !token.IsIdentifier(symbol.Name) {
			return nil, nil, fmt.Errorf("backend target must use exact Go identifiers")
		}
		if (symbol.Receiver == nil && symbol.Kind != model.SymbolFunction) || (symbol.Receiver != nil && (symbol.Kind != model.SymbolMethod || !token.IsIdentifier(symbol.Receiver.Type))) {
			return nil, nil, fmt.Errorf("backend target has invalid symbol kind or receiver")
		}
		if symbol.PackageName == "main" {
			return nil, nil, fmt.Errorf("main package selectors require a verified isolated build mapping")
		}
		if symbol.PackageImportPath == hookImportPath {
			return nil, nil, fmt.Errorf("generated hooks cannot instrument their own package")
		}
		receiver := ""
		if symbol.Receiver != nil {
			receiver = symbol.Receiver.Type
			if symbol.Receiver.Pointer {
				receiver = "*" + receiver
			}
			if parsed.Receiver == nil || parsed.Receiver.Type != symbol.Receiver.Type || parsed.Receiver.Pointer != symbol.Receiver.Pointer {
				return nil, nil, fmt.Errorf("backend target has inconsistent receiver identity")
			}
		} else if parsed.Receiver != nil {
			return nil, nil, fmt.Errorf("backend target has inconsistent receiver identity")
		}
		sum := sha256.Sum256([]byte(target.SymbolID))
		suffix := fmt.Sprintf("%x", sum[:8])
		if seen[suffix] {
			return nil, nil, fmt.Errorf("duplicate backend target or generated rule identity")
		}
		seen[suffix] = true
		binding := HookBinding{Symbol: target.SymbolID, Before: "Before_" + suffix, After: "After_" + suffix}
		rule := functionRule{Target: symbol.PackageImportPath, Where: functionSelector{Function: symbol.Name, Receiver: receiver}, Actions: []hookAction{{Hooks: hookAdvice{Before: binding.Before, After: binding.After, Path: hookImportPath}}}}
		var node yaml.Node
		if err := node.Encode(rule); err != nil {
			return nil, nil, fmt.Errorf("encode backend rule: %w", err)
		}
		key := yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "otelplan_" + suffix, HeadComment: "policy rule " + strconv.Quote(target.RuleID) + "; symbol " + strconv.Quote(string(target.SymbolID))}
		document.Content = append(document.Content, &key, &node)
		bindings = append(bindings, binding)
	}
	data, err := yaml.Marshal(&document)
	if err != nil {
		return nil, nil, fmt.Errorf("encode backend rules: %w", err)
	}
	return data, bindings, nil
}
