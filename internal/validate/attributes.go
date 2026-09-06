package validate

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

type Accessor struct {
	Source string   `json:"source"`
	Index  int      `json:"index"`
	Fields []string `json:"fields,omitempty"`
	Type   string   `json:"type"`
	Kind   string   `json:"kind"`
}

func AttributeAccessor(code *model.CodeModel, symbol model.Symbol, source model.AttributeSource) (Accessor, error) {
	sources := 0
	for _, present := range []bool{source.Argument != "", source.Result != "", source.Constant != nil} {
		if present {
			sources++
		}
	}
	if sources != 1 {
		return Accessor{}, fmt.Errorf("attribute must have exactly one source")
	}
	if source.Constant != nil {
		kind := ""
		switch source.Constant.(type) {
		case bool:
			kind = "bool"
		case string:
			kind = "string"
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			kind = "integer"
		case float32, float64:
			kind = "float"
		}
		if kind == "" {
			return Accessor{}, fmt.Errorf("constant must be a scalar")
		}
		switch value := source.Constant.(type) {
		case float64:
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return Accessor{}, fmt.Errorf("constant must be finite")
			}
		case float32:
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return Accessor{}, fmt.Errorf("constant must be finite")
			}
		case uint64:
			if value > math.MaxInt64 {
				return Accessor{}, fmt.Errorf("integer constant exceeds telemetry range")
			}
		case uint:
			if uint64(value) > math.MaxInt64 {
				return Accessor{}, fmt.Errorf("integer constant exceeds telemetry range")
			}
		}
		return Accessor{Source: "constant", Kind: kind}, nil
	}
	access := Accessor{Source: "argument", Index: -1}
	path := source.Argument
	var names, types []string
	if path != "" {
		for _, param := range symbol.Parameters {
			names = append(names, param.Name)
			types = append(types, param.Type)
		}
	} else {
		access.Source = "result"
		path = source.Result
		for _, result := range symbol.Results {
			names = append(names, result.Name)
			types = append(types, result.Type)
		}
	}
	parts := strings.Split(path, ".")
	for i, name := range names {
		if name != "" && name == parts[0] {
			access.Index = i
			break
		}
	}
	if access.Index < 0 {
		if index, err := strconv.Atoi(parts[0]); err == nil && index >= 0 && index < len(names) {
			access.Index = index
		}
	}
	if access.Index < 0 {
		return Accessor{}, fmt.Errorf("attribute source parameter or result does not exist")
	}
	typeIndex := map[string]model.TypeInfo{}
	if code == nil {
		return Accessor{}, fmt.Errorf("attribute source type is unavailable")
	}
	for _, typ := range code.Types {
		typeIndex[typ.Type] = typ
	}
	current := types[access.Index]
	for _, name := range parts[1:] {
		fields, err := lookupField(typeIndex, current, name)
		if err != nil {
			return Accessor{}, err
		}
		for _, field := range fields {
			access.Fields = append(access.Fields, field.Name)
			current = field.Type
		}
	}
	access.Type = current
	typ, ok := typeIndex[current]
	if !ok {
		return Accessor{}, fmt.Errorf("attribute source type is unavailable")
	}
	switch typ.Kind {
	case "bool", "integer", "float", "string":
		access.Kind = typ.Kind
	default:
		return Accessor{}, fmt.Errorf("attribute source must be a scalar; objects, collections, pointers, and interfaces cannot be captured")
	}
	return access, nil
}

func lookupField(index map[string]model.TypeInfo, root, name string) ([]model.TypeField, error) {
	type candidate struct {
		typ     string
		path    []model.TypeField
		visited map[string]bool
	}
	layer := []candidate{{typ: root, visited: map[string]bool{}}}
	for len(layer) > 0 {
		var next []candidate
		var matches [][]model.TypeField
		for _, entry := range layer {
			if entry.visited[entry.typ] {
				continue
			}
			visited := map[string]bool{}
			for typ := range entry.visited {
				visited[typ] = true
			}
			visited[entry.typ] = true
			shape, ok := index[entry.typ]
			if ok && shape.Kind == "pointer" {
				shape, ok = index[shape.Element]
			}
			if !ok || shape.Kind != "struct" {
				continue
			}
			for _, field := range shape.Fields {
				if !field.Exported {
					continue
				}
				path := append(append([]model.TypeField(nil), entry.path...), field)
				if field.Name == name {
					matches = append(matches, path)
				}
				if field.Embedded {
					next = append(next, candidate{typ: field.Type, path: path, visited: visited})
				}
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			return nil, fmt.Errorf("attribute field is ambiguous")
		}
		layer = next
	}
	return nil, fmt.Errorf("attribute field is missing or inaccessible")
}
