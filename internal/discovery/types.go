package discovery

import (
	"crypto/sha256"
	"fmt"
	"go/types"
	"sort"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const unsupportedTypeKind = "unsupported"

func (indexer *builder) collectType(typ types.Type) string {
	name := types.TypeString(typ, nil)
	if _, exists := indexer.types[name]; exists {
		return name
	}

	var info model.TypeInfo

	info.Type, info.Kind = name, unsupportedTypeKind
	// Reserve before traversing fields so recursive types terminate.
	indexer.types[name] = info
	info.Expression, info.Imports = typeExpression(typ)
	indexer.classifyType(typ, &info)
	indexer.types[name] = info

	return name
}

func typeExpression(typ types.Type) (string, []model.TypeImport) {
	imports := map[string]string{}
	expression := types.TypeString(typ, func(pkg *types.Package) string {
		digest := sha256.Sum256([]byte(pkg.Path()))
		alias := fmt.Sprintf("otelplanpkg_%x", digest[:8])
		imports[pkg.Path()] = alias

		return alias
	})

	if len(imports) == 0 {
		return expression, nil
	}

	dependencies := make([]model.TypeImport, 0, len(imports))

	for path, alias := range imports {
		dependencies = append(dependencies, model.TypeImport{Path: path, Alias: alias})
	}

	sort.Slice(dependencies, func(i, j int) bool { return dependencies[i].Path < dependencies[j].Path })

	return expression, dependencies
}

func (indexer *builder) classifyType(typ types.Type, info *model.TypeInfo) {
	switch underlying := types.Unalias(typ).Underlying().(type) {
	case *types.Basic:
		info.Kind = basicTypeKind(underlying)
	case *types.Pointer:
		info.Kind = "pointer"
		info.Element = indexer.collectType(underlying.Elem())
	case *types.Struct:
		info.Kind = "struct"
		info.Fields = indexer.collectTypeFields(underlying)
	case *types.Interface:
		info.Kind = "interface"
	case *types.Slice:
		info.Kind = "slice"
	case *types.Map:
		info.Kind = "map"
	case *types.Array:
		info.Kind = "array"
	case *types.Signature:
		info.Kind = "function"
	case *types.Chan:
		info.Kind = "channel"
	}
}

func basicTypeKind(basic *types.Basic) string {
	switch {
	case basic.Info()&types.IsBoolean != 0:
		return "bool"
	case basic.Info()&types.IsInteger != 0:
		return "integer"
	case basic.Info()&types.IsFloat != 0:
		return "float"
	case basic.Info()&types.IsString != 0:
		return "string"
	default:
		return unsupportedTypeKind
	}
}

func (indexer *builder) collectTypeFields(structure *types.Struct) []model.TypeField {
	var fields []model.TypeField

	for field := range structure.Fields() {
		fields = append(fields, model.TypeField{
			Name: field.Name(), Type: indexer.collectType(field.Type()), Exported: field.Exported(), Embedded: field.Embedded(),
		})
	}

	return fields
}
