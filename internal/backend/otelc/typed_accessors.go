package otelc

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/module"
	"golang.org/x/tools/go/ast/astutil"
)

type AccessorBinding struct {
	Key      string
	Function string
	Source   string
	Index    int
	Kind     string
}

// RenderAccessors emits compile-only helpers for injection into the target package.
// Helpers return primitive scalars and an availability flag without reflection.
func RenderAccessors(code *model.CodeModel, target model.ResolvedTarget) ([]byte, []AccessorBinding, error) {
	plan := model.ResolvedPlan{Targets: []model.ResolvedTarget{target}}
	if diagnostics := Check(SupportedVersion, code, plan); diagnostics.HasErrors() {
		return nil, nil, fmt.Errorf("accessor target is incompatible: %s", diagnostics.Errors()[0].Message)
	}
	if diagnostics := validate.Safety(code, plan, validate.Options{}); diagnostics.HasErrors() {
		return nil, nil, fmt.Errorf("unsafe attribute plan: %s", diagnostics.Errors()[0].Message)
	}
	symbol, _ := code.Symbol(target.SymbolID)
	parsed, err := model.ParseSymbolID(symbol.ID)
	if err != nil || parsed.ImportPath != symbol.PackageImportPath || parsed.Name != symbol.Name || !token.IsIdentifier(symbol.PackageName) || symbol.PackageName == "_" {
		return nil, nil, fmt.Errorf("accessor target has invalid package identity")
	}
	types := make(map[string]model.TypeInfo, len(code.Types))
	for _, typ := range code.Types {
		types[typ.Type] = typ
	}
	declarations := map[string]bool{}
	for _, candidate := range code.Symbols {
		if candidate.PackageImportPath == symbol.PackageImportPath && candidate.Receiver == nil {
			declarations[candidate.Name] = true
		}
	}
	attributes := append([]model.AttributePlan(nil), target.Attributes...)
	sort.Slice(attributes, func(i, j int) bool { return attributes[i].Key < attributes[j].Key })
	imports := map[string]string{}
	var body bytes.Buffer
	bindings := make([]AccessorBinding, 0, len(attributes))
	seen := map[string]bool{}
	names := map[string]bool{}
	for _, attribute := range attributes {
		if strings.TrimSpace(attribute.Key) == "" || strings.HasPrefix(strings.ToLower(attribute.Key), "otel.") || seen[attribute.Key] {
			return nil, nil, fmt.Errorf("attribute keys must be unique, nonempty, and outside the reserved otel. namespace")
		}
		seen[attribute.Key] = true
		access, err := validate.AttributeAccessor(code, *symbol, attribute.From)
		if err != nil {
			return nil, nil, err
		}
		digest := sha256.Sum256([]byte(string(symbol.ID) + "\x00" + attribute.Key))
		name := fmt.Sprintf("OTelPlanAttribute_%x", digest[:8])
		if names[name] {
			return nil, nil, fmt.Errorf("generated accessor name collision")
		}
		names[name] = true
		if declarations[name] {
			return nil, nil, fmt.Errorf("generated accessor collides with an existing declaration")
		}
		resultType, zero := scalarType(access.Kind)
		binding := AccessorBinding{Key: attribute.Key, Function: name, Source: access.Source, Index: access.Index, Kind: access.Kind}
		if access.Kind == "float" {
			if path, exists := imports["otelplanmath"]; exists && path != "math" {
				return nil, nil, fmt.Errorf("attribute type import alias collision")
			}
			imports["otelplanmath"] = "math"
		}
		if access.Source == "constant" {
			binding.Index = -1
			fmt.Fprintf(&body, "func %s(_ any) (%s, bool) { return %s, true }\n", name, resultType, scalarLiteral(attribute.From.Constant))
		} else {
			rootType := ""
			if access.Source == "argument" {
				rootType = symbol.Parameters[access.Index].Type
			} else {
				rootType = symbol.Results[access.Index].Type
			}
			typeExpr, err := accessorTypeExpression(types[rootType], symbol.PackageImportPath, imports)
			if err != nil {
				return nil, nil, err
			}
			fmt.Fprintf(&body, "func %s(value any) (%s, bool) {\ntyped, ok := value.(%s)\nif !ok { return %s, false }\n", name, resultType, typeExpr, zero)
			current, expression := rootType, "typed"
			for fieldIndex, fieldName := range access.Fields {
				if !token.IsIdentifier(fieldName) || !ast.IsExported(fieldName) {
					return nil, nil, fmt.Errorf("attribute fields must be exported Go identifiers")
				}
				shape := types[current]
				if shape.Kind == "pointer" {
					pointer := fmt.Sprintf("pointer%d", fieldIndex)
					fmt.Fprintf(&body, "%s := %s\nif %s == nil { return %s, false }\n", pointer, expression, pointer, zero)
					expression = pointer
					shape = types[shape.Element]
				}
				found := false
				for _, field := range shape.Fields {
					if field.Name == fieldName {
						found = true
						current = field.Type
						break
					}
				}
				if !found {
					return nil, nil, fmt.Errorf("attribute field type is unavailable")
				}
				expression += "." + fieldName
			}
			fmt.Fprintf(&body, "scalarValue := %s\nconverted := %s(scalarValue)\n", expression, resultType)
			switch access.Kind {
			case "integer":
				body.WriteString("if scalarValue > 0 && converted < 0 { return 0, false }\n")
			case "float":
				body.WriteString("if otelplanmath.IsNaN(converted) || otelplanmath.IsInf(converted, 0) { return 0, false }\n")
			}
			body.WriteString("return converted, true\n}\n")
		}
		bindings = append(bindings, binding)
	}
	var source bytes.Buffer
	fmt.Fprintf(&source, "//go:build ignore\n\n// Code generated by OTelPlan. DO NOT EDIT.\n\npackage %s\n", symbol.PackageName)
	if len(imports) > 0 {
		aliases := make([]string, 0, len(imports))
		for alias := range imports {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		source.WriteString("import (\n")
		for _, alias := range aliases {
			fmt.Fprintf(&source, "%s %q\n", alias, imports[alias])
		}
		source.WriteString(")\n")
	}
	source.Write(body.Bytes())
	data, err := format.Source(source.Bytes())
	if err != nil {
		return nil, nil, fmt.Errorf("format accessors: %w", err)
	}
	return data, bindings, nil
}

func accessorTypeExpression(typ model.TypeInfo, ownPackage string, imports map[string]string) (string, error) {
	if typ.Expression == "" {
		return "", fmt.Errorf("attribute source is missing its Go type expression")
	}
	expression, err := parser.ParseExpr(typ.Expression)
	if err != nil {
		return "", fmt.Errorf("invalid attribute source type expression")
	}
	ownAliases := map[string]bool{}
	for _, dependency := range typ.Imports {
		if !token.IsIdentifier(dependency.Alias) || dependency.Alias == "_" || module.CheckImportPath(dependency.Path) != nil {
			return "", fmt.Errorf("invalid attribute type import")
		}
		if dependency.Path == ownPackage {
			ownAliases[dependency.Alias] = true
			continue
		}
		if previous, exists := imports[dependency.Alias]; exists && previous != dependency.Path {
			return "", fmt.Errorf("attribute type import alias collision")
		}
		imports[dependency.Alias] = dependency.Path
	}
	expression = astutil.Apply(expression, func(cursor *astutil.Cursor) bool {
		selector, ok := cursor.Node().(*ast.SelectorExpr)
		if !ok {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if ok && ownAliases[qualifier.Name] {
			cursor.Replace(selector.Sel)
			return false
		}
		return true
	}, nil).(ast.Expr)
	var source bytes.Buffer
	if err := format.Node(&source, token.NewFileSet(), expression); err != nil {
		return "", fmt.Errorf("format attribute type expression: %w", err)
	}
	return source.String(), nil
}

func scalarType(kind string) (name, zero string) {
	switch kind {
	case "bool":
		return "bool", "false"
	case "string":
		return "string", `""`
	case "float":
		return "float64", "0"
	default:
		return "int64", "0"
	}
}

func scalarLiteral(value any) string {
	switch scalar := value.(type) {
	case string:
		return strconv.Quote(scalar)
	case float32:
		return fmt.Sprintf("otelplanmath.Float64frombits(%d)", math.Float64bits(float64(scalar)))
	case float64:
		return fmt.Sprintf("otelplanmath.Float64frombits(%d)", math.Float64bits(scalar))
	default:
		return fmt.Sprint(value)
	}
}
