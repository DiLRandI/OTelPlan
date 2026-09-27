// Package resolve selects instrumentation targets from a semantic Go code model.
package resolve

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

// Glob treats ** as zero or more path segments; * and ? never cross a slash.
func Glob(pattern, value string) (bool, error) {
	parts, err := validatedGlobParts(pattern)
	if err != nil {
		return false, err
	}

	values := strings.Split(value, "/")

	type position struct{ pattern, value int }

	memo := map[position]bool{}
	visited := map[position]bool{}

	var match func(int, int) bool

	match = func(patternIndex, valueIndex int) bool {
		key := position{patternIndex, valueIndex}
		if visited[key] {
			return memo[key]
		}

		visited[key] = true

		result := false

		switch {
		case patternIndex == len(parts):
			result = valueIndex == len(values)
		case parts[patternIndex] == "**":
			result = match(patternIndex+1, valueIndex) || (valueIndex < len(values) && match(patternIndex, valueIndex+1))
		case valueIndex < len(values):
			ok, _ := path.Match(parts[patternIndex], values[valueIndex])
			result = ok && match(patternIndex+1, valueIndex+1)
		}

		memo[key] = result

		return result
	}

	return match(0, 0), nil
}

func validatedGlobParts(pattern string) ([]string, error) {
	parts := strings.Split(pattern, "/")
	for _, part := range parts {
		if part == "**" {
			continue
		}

		_, err := path.Match(part, "")
		if err != nil {
			return nil, fmt.Errorf("invalid glob %q: %w", pattern, err)
		}
	}

	return parts, nil
}

// Matches applies selector fields as conjunctions and values within each field as alternatives.
// Invalid glob patterns return an error even when another field does not match.
func Matches(code *model.CodeModel, symbol model.Symbol, selector model.Match) (bool, error) {
	matched, err := matchesGlobFields(symbol, selector)
	if err != nil {
		return false, err
	}

	if len(selector.Symbols) > 0 {
		matched = matched && slices.Contains(selector.Symbols, string(symbol.ID))
	}

	if len(selector.Implements) > 0 {
		matched = matched && matchesInterfaceMethod(code, symbol.ID, selector.Implements)
	}

	return matched && matchesProperties(symbol, selector), nil
}

func matchesGlobFields(symbol model.Symbol, selector model.Match) (bool, error) {
	receiver, function, method := "", "", ""
	if symbol.Receiver != nil {
		receiver = symbol.Receiver.Type
	}

	if symbol.Kind == model.SymbolFunction {
		function = symbol.Name
	}

	if symbol.Kind == model.SymbolMethod {
		method = symbol.Name
	}

	fields := []struct {
		patterns []string
		value    string
	}{
		{selector.Packages, symbol.PackageImportPath},
		{selector.Files, symbol.Location.File},
		{selector.Functions, function},
		{selector.Methods, method},
		{selector.Receivers, receiver},
	}
	matched := true

	for _, field := range fields {
		fieldMatch, err := matchesPatterns(field.patterns, field.value)
		if err != nil {
			return false, err
		}

		matched = matched && fieldMatch
	}

	return matched, nil
}

func matchesInterfaceMethod(code *model.CodeModel, symbolID model.SymbolID, interfaces []string) bool {
	for _, binding := range code.InterfaceMethods {
		if binding.SymbolID == symbolID && slices.Contains(interfaces, string(binding.InterfaceID)) {
			return true
		}
	}

	return false
}

func matchesProperties(symbol model.Symbol, selector model.Match) bool {
	if selector.Exported != nil && *selector.Exported != (symbol.Visibility == model.VisibilityExported) {
		return false
	}

	if selector.HasContext != nil && *selector.HasContext != symbol.HasContext() {
		return false
	}

	if selector.ReturnsError != nil && *selector.ReturnsError != symbol.ReturnsError() {
		return false
	}

	return selector.Ownership == "" || selector.Ownership == model.OwnershipAny || selector.Ownership == symbol.Ownership
}

func matchesPatterns(patterns []string, value string) (bool, error) {
	if len(patterns) == 0 {
		return true, nil
	}

	matched := false

	for _, pattern := range patterns {
		patternMatch, err := Glob(pattern, value)
		if err != nil {
			return false, err
		}

		matched = matched || (patternMatch && value != "")
	}

	return matched, nil
}
