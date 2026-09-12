package discovery

import (
	"crypto/sha256"
	"fmt"
	"go/types"
	"sort"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func (b *builder) collectType(typ types.Type) string {
	name := types.TypeString(typ, nil)
	if _, ok := b.types[name]; ok {
		return name
	}
	// Reserve before traversing fields so recursive types terminate.
	b.types[name] = model.TypeInfo{Type: name, Kind: "unsupported"}
	info := model.TypeInfo{Type: name, Kind: "unsupported"}
	imports := map[string]string{}
	info.Expression = types.TypeString(typ, func(pkg *types.Package) string {
		digest := sha256.Sum256([]byte(pkg.Path()))
		alias := fmt.Sprintf("otelplanpkg_%x", digest[:8])
		imports[pkg.Path()] = alias
		return alias
	})
	for path, alias := range imports {
		info.Imports = append(info.Imports, model.TypeImport{Path: path, Alias: alias})
	}
	sort.Slice(info.Imports, func(i, j int) bool { return info.Imports[i].Path < info.Imports[j].Path })
	switch underlying := types.Unalias(typ).Underlying().(type) {
	case *types.Basic:
		switch {
		case underlying.Info()&types.IsBoolean != 0:
			info.Kind = "bool"
		case underlying.Info()&types.IsInteger != 0:
			info.Kind = "integer"
		case underlying.Info()&types.IsFloat != 0:
			info.Kind = "float"
		case underlying.Info()&types.IsString != 0:
			info.Kind = "string"
		}
	case *types.Pointer:
		info.Kind = "pointer"
		info.Element = b.collectType(underlying.Elem())
	case *types.Struct:
		info.Kind = "struct"
		for i := 0; i < underlying.NumFields(); i++ {
			field := underlying.Field(i)
			info.Fields = append(info.Fields, model.TypeField{Name: field.Name(), Type: b.collectType(field.Type()), Exported: field.Exported(), Embedded: field.Embedded()})
		}
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
	b.types[name] = info
	return name
}
