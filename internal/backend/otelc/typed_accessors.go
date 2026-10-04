package otelc

import (
	"bytes"
	"crypto/sha256"
	"errors"
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

const (
	accessorFloatKind  = "float"
	accessorBoolKind   = "bool"
	accessorStringKind = "string"
)

var (
	errAccessorIncompatible    = errors.New("accessor target is incompatible")
	errAccessorUnsafePlan      = errors.New("unsafe attribute plan")
	errAccessorPackageIdentity = errors.New("accessor target has invalid package identity")
	errAccessorAttributeKey    = errors.New(
		"attribute keys must be unique, nonempty, and outside the reserved otel. namespace",
	)
	errAccessorNameCollision        = errors.New("generated accessor name collision")
	errAccessorDeclarationCollision = errors.New("generated accessor collides with an existing declaration")
	errAccessorImportCollision      = errors.New("attribute type import alias collision")
	errAccessorFieldIdentifier      = errors.New("attribute fields must be exported Go identifiers")
	errAccessorFieldType            = errors.New("attribute field type is unavailable")
	errAccessorTypeMissing          = errors.New("attribute source is missing its Go type expression")
	errAccessorTypeExpression       = errors.New("invalid attribute source type expression")
	errAccessorTypeImport           = errors.New("invalid attribute type import")
)

// AccessorBinding identifies a generated scalar reader and its argument, result, or constant source.
type AccessorBinding struct {
	Key      string
	Function string
	Source   string
	Index    int
	Kind     string
}

type accessorRenderer struct {
	symbol       *model.Symbol
	types        map[string]model.TypeInfo
	declarations map[string]bool
	imports      map[string]string
	keys         map[string]bool
	names        map[string]bool
	body         bytes.Buffer
}

// RenderAccessors emits compile-only helpers for injection into the target package.
// Helpers return primitive scalars and an availability flag without reflection.
func RenderAccessors(code *model.CodeModel, target model.ResolvedTarget) ([]byte, []AccessorBinding, error) {
	symbol, err := validateAccessorTarget(code, target)
	if err != nil {
		return nil, nil, err
	}

	renderer := newAccessorRenderer(code, symbol)

	attributes := append([]model.AttributePlan(nil), target.Attributes...)
	sort.Slice(attributes, func(i, j int) bool { return attributes[i].Key < attributes[j].Key })
	bindings := make([]AccessorBinding, 0, len(attributes))

	for _, attribute := range attributes {
		binding, err := renderer.renderAttribute(code, attribute)
		if err != nil {
			return nil, nil, err
		}

		bindings = append(bindings, binding)
	}

	data, err := renderer.source()
	if err != nil {
		return nil, nil, err
	}

	return data, bindings, nil
}

func validateAccessorTarget(code *model.CodeModel, target model.ResolvedTarget) (*model.Symbol, error) {
	var plan model.ResolvedPlan

	plan.Targets = []model.ResolvedTarget{target}

	diagnostics := Check(SupportedVersion, code, plan)
	if diagnostics.HasErrors() {
		return nil, fmt.Errorf("%w: %s", errAccessorIncompatible, diagnostics.Errors()[0].Message)
	}

	var options validate.Options

	diagnostics = validate.Safety(code, plan, options)
	if diagnostics.HasErrors() {
		return nil, fmt.Errorf("%w: %s", errAccessorUnsafePlan, diagnostics.Errors()[0].Message)
	}

	symbol, _ := code.Symbol(target.SymbolID)
	parsed, err := model.ParseSymbolID(symbol.ID)

	if err != nil || parsed.ImportPath != symbol.PackageImportPath || parsed.Name != symbol.Name ||
		!token.IsIdentifier(symbol.PackageName) || symbol.PackageName == "_" {
		return nil, errAccessorPackageIdentity
	}

	return symbol, nil
}

func newAccessorRenderer(code *model.CodeModel, symbol *model.Symbol) *accessorRenderer {
	renderer := new(accessorRenderer)
	renderer.symbol = symbol
	renderer.types = make(map[string]model.TypeInfo, len(code.Types))
	renderer.declarations = map[string]bool{}
	renderer.imports = map[string]string{}
	renderer.keys = map[string]bool{}
	renderer.names = map[string]bool{}

	for _, typ := range code.Types {
		renderer.types[typ.Type] = typ
	}

	for _, candidate := range code.Symbols {
		if candidate.PackageImportPath == symbol.PackageImportPath && candidate.Receiver == nil {
			renderer.declarations[candidate.Name] = true
		}
	}

	return renderer
}

func (renderer *accessorRenderer) renderAttribute(
	code *model.CodeModel, attribute model.AttributePlan,
) (AccessorBinding, error) {
	var binding AccessorBinding

	if strings.TrimSpace(attribute.Key) == "" || strings.HasPrefix(strings.ToLower(attribute.Key), "otel.") ||
		renderer.keys[attribute.Key] {
		return binding, errAccessorAttributeKey
	}

	renderer.keys[attribute.Key] = true

	access, err := validate.AttributeAccessor(code, *renderer.symbol, attribute.From)
	if err != nil {
		return binding, fmt.Errorf("resolve attribute accessor: %w", err)
	}

	name, err := renderer.accessorName(attribute.Key)
	if err != nil {
		return binding, err
	}

	binding = AccessorBinding{
		Key: attribute.Key, Function: name, Source: access.Source, Index: access.Index, Kind: access.Kind,
	}

	err = renderer.addScalarImports(access.Kind)
	if err != nil {
		return binding, err
	}

	if access.Source == "constant" {
		binding.Index = -1
		resultType, _ := scalarType(access.Kind)
		fmt.Fprintf(&renderer.body, "func %s(_ any) (%s, bool) { return %s, true }\n",
			name, resultType, scalarLiteral(attribute.From.Constant))

		return binding, nil
	}

	err = renderer.renderValueAccessor(binding, access)

	return binding, err
}

func (renderer *accessorRenderer) accessorName(key string) (string, error) {
	digest := sha256.Sum256([]byte(string(renderer.symbol.ID) + "\x00" + key))
	name := fmt.Sprintf("OTelPlanAttribute_%x", digest[:8])

	if renderer.names[name] {
		return "", errAccessorNameCollision
	}

	renderer.names[name] = true

	if renderer.declarations[name] {
		return "", errAccessorDeclarationCollision
	}

	return name, nil
}

func (renderer *accessorRenderer) addScalarImports(kind string) error {
	if kind != accessorFloatKind {
		return nil
	}

	if path, exists := renderer.imports["otelplanmath"]; exists && path != "math" {
		return errAccessorImportCollision
	}

	renderer.imports["otelplanmath"] = "math"

	return nil
}

func (renderer *accessorRenderer) renderValueAccessor(binding AccessorBinding, access validate.Accessor) error {
	var typeName string

	if access.Source == "argument" {
		typeName = renderer.symbol.Parameters[access.Index].Type
	} else {
		typeName = renderer.symbol.Results[access.Index].Type
	}

	typeExpression, err := accessorTypeExpression(renderer.types[typeName], renderer.symbol.PackageImportPath,
		renderer.imports)
	if err != nil {
		return err
	}

	resultType, zero := scalarType(access.Kind)
	fmt.Fprintf(&renderer.body, "func %s(value any) (%s, bool) {\ntyped, ok := value.(%s)\n"+
		"if !ok { return %s, false }\n", binding.Function, resultType, typeExpression, zero)

	expression, err := renderer.fieldExpression(typeName, access.Fields, zero)
	if err != nil {
		return err
	}

	fmt.Fprintf(&renderer.body, "scalarValue := %s\nconverted := %s(scalarValue)\n", expression, resultType)

	switch access.Kind {
	case "integer":
		renderer.body.WriteString("if scalarValue > 0 && converted < 0 { return 0, false }\n")
	case accessorFloatKind:
		renderer.body.WriteString(
			"if otelplanmath.IsNaN(converted) || otelplanmath.IsInf(converted, 0) { return 0, false }\n",
		)
	}

	renderer.body.WriteString("return converted, true\n}\n")

	return nil
}

func (renderer *accessorRenderer) fieldExpression(current string, fields []string, zero string) (string, error) {
	expression := "typed"

	for fieldIndex, fieldName := range fields {
		if !token.IsIdentifier(fieldName) || !ast.IsExported(fieldName) {
			return "", errAccessorFieldIdentifier
		}

		shape := renderer.types[current]
		if shape.Kind == "pointer" {
			pointer := fmt.Sprintf("pointer%d", fieldIndex)
			fmt.Fprintf(&renderer.body, "%s := %s\nif %s == nil { return %s, false }\n",
				pointer, expression, pointer, zero)
			expression = pointer
			shape = renderer.types[shape.Element]
		}

		fieldType, err := accessorFieldType(shape, fieldName)
		if err != nil {
			return "", err
		}

		current = fieldType
		expression += "." + fieldName
	}

	return expression, nil
}

func accessorFieldType(shape model.TypeInfo, name string) (string, error) {
	for _, field := range shape.Fields {
		if field.Name == name {
			return field.Type, nil
		}
	}

	return "", errAccessorFieldType
}

func (renderer *accessorRenderer) source() ([]byte, error) {
	var source bytes.Buffer

	fmt.Fprintf(&source, "//go:build ignore\n\n// Code generated by OTelPlan. DO NOT EDIT.\n\npackage %s\n",
		renderer.symbol.PackageName)

	if len(renderer.imports) > 0 {
		aliases := make([]string, 0, len(renderer.imports))
		for alias := range renderer.imports {
			aliases = append(aliases, alias)
		}

		sort.Strings(aliases)
		source.WriteString("import (\n")

		for _, alias := range aliases {
			fmt.Fprintf(&source, "%s %q\n", alias, renderer.imports[alias])
		}

		source.WriteString(")\n")
	}

	source.Write(renderer.body.Bytes())

	data, err := format.Source(source.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format accessors: %w", err)
	}

	return data, nil
}

func accessorTypeExpression(typ model.TypeInfo, ownPackage string, imports map[string]string) (string, error) {
	if typ.Expression == "" {
		return "", errAccessorTypeMissing
	}

	expression, err := parser.ParseExpr(typ.Expression)
	if err != nil {
		return "", errAccessorTypeExpression
	}

	ownAliases, err := accessorTypeImports(typ.Imports, ownPackage, imports)
	if err != nil {
		return "", err
	}

	rewritten := astutil.Apply(expression, func(cursor *astutil.Cursor) bool {
		selector, isSelector := cursor.Node().(*ast.SelectorExpr)
		if !isSelector {
			return true
		}

		qualifier, isIdentifier := selector.X.(*ast.Ident)
		if isIdentifier && ownAliases[qualifier.Name] {
			cursor.Replace(selector.Sel)

			return false
		}

		return true
	}, nil)

	var source bytes.Buffer

	err = format.Node(&source, token.NewFileSet(), rewritten)
	if err != nil {
		return "", fmt.Errorf("format attribute type expression: %w", err)
	}

	return source.String(), nil
}

func accessorTypeImports(dependencies []model.TypeImport, ownPackage string,
	imports map[string]string) (map[string]bool, error) {
	ownAliases := map[string]bool{}

	for _, dependency := range dependencies {
		if !token.IsIdentifier(dependency.Alias) || dependency.Alias == "_" ||
			module.CheckImportPath(dependency.Path) != nil {
			return nil, errAccessorTypeImport
		}

		if dependency.Path == ownPackage {
			ownAliases[dependency.Alias] = true

			continue
		}

		if previous, exists := imports[dependency.Alias]; exists && previous != dependency.Path {
			return nil, errAccessorImportCollision
		}

		imports[dependency.Alias] = dependency.Path
	}

	return ownAliases, nil
}

func scalarType(kind string) (string, string) {
	switch kind {
	case accessorBoolKind:
		return accessorBoolKind, "false"
	case accessorStringKind:
		return accessorStringKind, `""`
	case accessorFloatKind:
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
