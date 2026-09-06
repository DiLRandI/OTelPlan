package resolve

import (
	"fmt"
	"path"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

// Glob treats ** as zero or more path segments; * and ? never cross a slash.
func Glob(pattern, value string) (bool, error) {
	parts := strings.Split(pattern, "/")
	for _, part := range parts {
		if part == "**" {
			continue
		}
		if _, err := path.Match(part, ""); err != nil {
			return false, fmt.Errorf("invalid glob %q: %w", pattern, err)
		}
	}
	values := strings.Split(value, "/")
	type position struct{ pattern, value int }
	memo := map[position]bool{}
	visited := map[position]bool{}
	var match func(int, int) bool
	match = func(i, j int) bool {
		key := position{i, j}
		if visited[key] {
			return memo[key]
		}
		visited[key] = true
		result := false
		switch {
		case i == len(parts):
			result = j == len(values)
		case parts[i] == "**":
			result = match(i+1, j) || (j < len(values) && match(i, j+1))
		case j < len(values):
			ok, _ := path.Match(parts[i], values[j])
			result = ok && match(i+1, j+1)
		}
		memo[key] = result
		return result
	}
	return match(0, 0), nil
}

func Matches(m *model.CodeModel, s model.Symbol, selector model.Match) (bool, error) {
	receiver, function, method := "", "", ""
	if s.Receiver != nil {
		receiver = s.Receiver.Type
	}
	if s.Kind == model.SymbolFunction {
		function = s.Name
	}
	if s.Kind == model.SymbolMethod {
		method = s.Name
	}
	fields := []struct {
		patterns []string
		value    string
	}{
		{selector.Packages, s.PackageImportPath}, {selector.Files, s.Location.File},
		{selector.Functions, function}, {selector.Methods, method}, {selector.Receivers, receiver},
	}
	matched := true
	for _, field := range fields {
		if len(field.patterns) == 0 {
			continue
		}
		fieldMatch := false
		for _, pattern := range field.patterns {
			ok, err := Glob(pattern, field.value)
			if err != nil {
				return false, err
			}
			fieldMatch = fieldMatch || (ok && field.value != "")
		}
		matched = matched && fieldMatch
	}
	if len(selector.Symbols) > 0 {
		found := false
		for _, id := range selector.Symbols {
			found = found || id == string(s.ID)
		}
		matched = matched && found
	}
	if len(selector.Implements) > 0 {
		found := false
		for _, binding := range m.InterfaceMethods {
			if binding.SymbolID != s.ID {
				continue
			}
			for _, id := range selector.Implements {
				found = found || id == string(binding.InterfaceID)
			}
		}
		matched = matched && found
	}
	if selector.Exported != nil {
		matched = matched && *selector.Exported == (s.Visibility == model.VisibilityExported)
	}
	if selector.HasContext != nil {
		matched = matched && *selector.HasContext == s.HasContext()
	}
	if selector.ReturnsError != nil {
		matched = matched && *selector.ReturnsError == s.ReturnsError()
	}
	if selector.Ownership != "" && selector.Ownership != model.OwnershipAny {
		matched = matched && selector.Ownership == s.Ownership
	}
	return matched, nil
}
