package validate

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

var (
	errAttributeSourceCount     = errors.New("attribute must have exactly one source")
	errConstantScalar           = errors.New("constant must be a scalar")
	errConstantFinite           = errors.New("constant must be finite")
	errConstantRange            = errors.New("integer constant exceeds telemetry range")
	errAttributeSourceMissing   = errors.New("attribute source parameter or result does not exist")
	errAttributeTypeUnavailable = errors.New("attribute source type is unavailable")
	errAttributeScalar          = errors.New(
		"attribute source must be a scalar; objects, collections, pointers, and interfaces cannot be captured",
	)
	errAttributeFieldAmbiguous    = errors.New("attribute field is ambiguous")
	errAttributeFieldInaccessible = errors.New("attribute field is missing or inaccessible")
)

// Accessor describes a validated scalar attribute source. Index addresses the
// argument or result; Fields includes embedded fields needed to reach the value.
// Constant sources have no field path or Go type and use Kind for their scalar type.
type Accessor struct {
	Source string   `json:"source"`
	Index  int      `json:"index"`
	Fields []string `json:"fields,omitempty"`
	Type   string   `json:"type"`
	Kind   string   `json:"kind"`
}

// AttributeAccessor resolves exactly one constant, argument, or result source.
// Constants must be finite scalars within the telemetry integer range. Field
// access requires an unambiguous exported path ending in a scalar value.
// Argument and result sources require type information in code; constants do not.
func AttributeAccessor(code *model.CodeModel, symbol model.Symbol, source model.AttributeSource) (Accessor, error) {
	sources := 0

	for _, present := range []bool{source.Argument != "", source.Result != "", source.Constant != nil} {
		if present {
			sources++
		}
	}

	if sources != 1 {
		return Accessor{}, errAttributeSourceCount
	}

	if source.Constant != nil {
		return constantAccessor(source.Constant)
	}

	access, fields, err := sourceAccessor(symbol, source)
	if err != nil {
		return Accessor{}, err
	}

	return resolveAccessorFields(code, access, fields)
}

func sourceAccessor(symbol model.Symbol, source model.AttributeSource) (Accessor, []string, error) {
	access := Accessor{Source: "argument", Index: -1, Fields: nil, Type: "", Kind: ""}
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
	access.Index = sourceIndex(names, parts[0])

	if access.Index < 0 {
		return Accessor{}, nil, errAttributeSourceMissing
	}

	access.Type = types[access.Index]

	return access, parts[1:], nil
}

func sourceIndex(names []string, selector string) int {
	for index, name := range names {
		if name != "" && name == selector {
			return index
		}
	}

	index, err := strconv.Atoi(selector)
	if err != nil || index < 0 || index >= len(names) {
		return -1
	}

	return index
}

func resolveAccessorFields(code *model.CodeModel, access Accessor, parts []string) (Accessor, error) {
	typeIndex := map[string]model.TypeInfo{}

	if code == nil {
		return Accessor{}, errAttributeTypeUnavailable
	}

	for _, typ := range code.Types {
		typeIndex[typ.Type] = typ
	}

	current := access.Type
	for _, name := range parts {
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
		return Accessor{}, errAttributeTypeUnavailable
	}

	switch typ.Kind {
	case "bool", "integer", "float", "string":
		access.Kind = typ.Kind
	default:
		return Accessor{}, errAttributeScalar
	}

	return access, nil
}

func constantAccessor(constant any) (Accessor, error) {
	kind := constantKind(constant)
	if kind == "" {
		return Accessor{}, errConstantScalar
	}

	err := validateConstantRange(constant)
	if err != nil {
		return Accessor{}, err
	}

	return Accessor{Source: "constant", Index: 0, Fields: nil, Type: "", Kind: kind}, nil
}

func constantKind(constant any) string {
	switch constant.(type) {
	case bool:
		return "bool"
	case string:
		return "string"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "integer"
	case float32, float64:
		return "float"
	default:
		return ""
	}
}

func validateConstantRange(constant any) error {
	switch value := constant.(type) {
	case float64:
		if !finiteConstant(value) {
			return errConstantFinite
		}
	case float32:
		if !finiteConstant(float64(value)) {
			return errConstantFinite
		}
	case uint64:
		if value > math.MaxInt64 {
			return errConstantRange
		}
	case uint:
		if uint64(value) > math.MaxInt64 {
			return errConstantRange
		}
	}

	return nil
}

func finiteConstant(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

type fieldCandidate struct {
	typ     string
	path    []model.TypeField
	visited map[string]bool
}

func lookupField(index map[string]model.TypeInfo, root, name string) ([]model.TypeField, error) {
	layer := []fieldCandidate{{typ: root, path: nil, visited: map[string]bool{}}}
	for len(layer) > 0 {
		var (
			next    []fieldCandidate
			matches [][]model.TypeField
		)

		for _, entry := range layer {
			children, found := expandFieldCandidate(index, entry, name)
			next = append(next, children...)
			matches = append(matches, found...)
		}

		if len(matches) == 1 {
			return matches[0], nil
		}

		if len(matches) > 1 {
			return nil, errAttributeFieldAmbiguous
		}

		layer = next
	}

	return nil, errAttributeFieldInaccessible
}

func expandFieldCandidate(
	index map[string]model.TypeInfo, entry fieldCandidate, name string,
) ([]fieldCandidate, [][]model.TypeField) {
	var (
		next    []fieldCandidate
		matches [][]model.TypeField
	)

	if entry.visited[entry.typ] {
		return nil, nil
	}

	visited := map[string]bool{}
	for typ := range entry.visited {
		visited[typ] = true
	}

	visited[entry.typ] = true

	shape, ok := fieldStruct(index, entry.typ)
	if !ok {
		return nil, nil
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
			next = append(next, fieldCandidate{typ: field.Type, path: path, visited: visited})
		}
	}

	return next, matches
}

func fieldStruct(index map[string]model.TypeInfo, typ string) (model.TypeInfo, bool) {
	shape, ok := index[typ]
	if ok && shape.Kind == "pointer" {
		shape, ok = index[shape.Element]
	}

	return shape, ok && shape.Kind == "struct"
}
