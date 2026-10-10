package otelc

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/token"
	"sort"
	"strconv"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/module"
	"gopkg.in/yaml.v3"
)

var (
	errRuleHookImportPath      = errors.New("invalid generated hook import path")
	errRulePlanIncompatible    = errors.New("plan is incompatible with the pinned backend")
	errRuleCanonicalIdentity   = errors.New("backend target has inconsistent canonical identity")
	errRuleGoIdentifiers       = errors.New("backend target must use exact Go identifiers")
	errRuleSymbolKind          = errors.New("backend target has invalid symbol kind or receiver")
	errRuleSelfInstrumentation = errors.New("generated hooks cannot instrument their own package")
	errRuleReceiverIdentity    = errors.New("backend target has inconsistent receiver identity")
	errDuplicateRuleIdentity   = errors.New("duplicate backend target or generated rule identity")
)

// HookBinding connects one canonical symbol to its generated before/after hooks.
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
func RenderRules(version string, code *model.CodeModel, plan model.ResolvedPlan,
	hookImportPath string) ([]byte, []HookBinding, error) {
	err := validateRulePlan(version, code, plan, hookImportPath)
	if err != nil {
		return nil, nil, err
	}

	targets := append([]model.ResolvedTarget(nil), plan.Targets...)
	sort.Slice(targets, func(i, j int) bool { return targets[i].SymbolID < targets[j].SymbolID })

	document := new(yaml.Node)
	document.Kind, document.Tag = yaml.MappingNode, "!!map"
	bindings := make([]HookBinding, 0, len(targets))
	seen := map[string]bool{}

	for _, target := range targets {
		symbol, _ := code.Symbol(target.SymbolID)

		receiver, err := exactRuleReceiver(target.SymbolID, symbol, hookImportPath)
		if err != nil {
			return nil, nil, err
		}

		sum := sha256.Sum256([]byte(target.SymbolID))

		suffix := hex.EncodeToString(sum[:8])
		if seen[suffix] {
			return nil, nil, errDuplicateRuleIdentity
		}

		seen[suffix] = true
		binding := HookBinding{Symbol: target.SymbolID, Before: "Before_" + suffix, After: "After_" + suffix}

		err = appendFunctionRule(document, target, symbol, receiver, binding, hookImportPath, suffix)
		if err != nil {
			return nil, nil, err
		}

		bindings = append(bindings, binding)
	}

	data, err := yaml.Marshal(document)
	if err != nil {
		return nil, nil, fmt.Errorf("encode backend rules: %w", err)
	}

	return data, bindings, nil
}

func validateRulePlan(version string, code *model.CodeModel, plan model.ResolvedPlan, hookImportPath string) error {
	err := module.CheckImportPath(hookImportPath)
	if err != nil {
		return errRuleHookImportPath
	}

	diagnostics := Check(version, code, plan)
	if diagnostics.HasErrors() {
		return fmt.Errorf("%w: %s", errRulePlanIncompatible, diagnostics.Errors()[0].Message)
	}

	return nil
}

func exactRuleReceiver(id model.SymbolID, symbol *model.Symbol, hookImportPath string) (string, error) {
	parsed, err := model.ParseSymbolID(id)
	if err != nil || parsed.ImportPath != symbol.PackageImportPath || parsed.Name != symbol.Name {
		return "", errRuleCanonicalIdentity
	}

	if module.CheckImportPath(symbol.PackageImportPath) != nil || !token.IsIdentifier(symbol.Name) {
		return "", errRuleGoIdentifiers
	}

	err = validateRuleSymbolKind(symbol)
	if err != nil {
		return "", err
	}

	if symbol.PackageImportPath == hookImportPath {
		return "", errRuleSelfInstrumentation
	}

	return matchingRuleReceiver(symbol, parsed.Receiver)
}

func validateRuleSymbolKind(symbol *model.Symbol) error {
	if symbol.Receiver == nil {
		if symbol.Kind != model.SymbolFunction {
			return errRuleSymbolKind
		}

		return nil
	}

	if symbol.Kind != model.SymbolMethod || !token.IsIdentifier(symbol.Receiver.Type) {
		return errRuleSymbolKind
	}

	return nil
}

func matchingRuleReceiver(symbol *model.Symbol, parsed *model.Receiver) (string, error) {
	if symbol.Receiver == nil {
		if parsed != nil {
			return "", errRuleReceiverIdentity
		}

		return "", nil
	}

	if parsed == nil || parsed.Type != symbol.Receiver.Type || parsed.Pointer != symbol.Receiver.Pointer {
		return "", errRuleReceiverIdentity
	}

	receiver := symbol.Receiver.Type
	if symbol.Receiver.Pointer {
		receiver = "*" + receiver
	}

	return receiver, nil
}

func appendFunctionRule(document *yaml.Node, target model.ResolvedTarget, symbol *model.Symbol, receiver string,
	binding HookBinding, hookImportPath, suffix string) error {
	rule := functionRule{
		Target: symbol.PackageImportPath, Where: functionSelector{Function: symbol.Name, Receiver: receiver},
		Actions: []hookAction{{Hooks: hookAdvice{Before: binding.Before, After: binding.After, Path: hookImportPath}}},
	}

	var node yaml.Node

	err := node.Encode(rule)
	if err != nil {
		return fmt.Errorf("encode backend rule: %w", err)
	}

	key := new(yaml.Node)
	key.Kind, key.Tag, key.Value = yaml.ScalarNode, "!!str", "otelplan_"+suffix
	key.HeadComment = "policy rule " + strconv.Quote(target.RuleID) + "; symbol " + strconv.Quote(string(target.SymbolID))
	document.Content = append(document.Content, key, &node)

	return nil
}
