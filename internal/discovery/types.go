package discovery

import (
	"go/types"

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
