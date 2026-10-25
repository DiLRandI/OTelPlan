package discovery

import "go/types"

func implementsStringer(typ types.Type) bool {
	method := types.NewMethodSet(typ).Lookup(nil, "String")
	if method == nil {
		return false
	}

	signature, ok := method.Obj().Type().(*types.Signature)
	if !ok || signature.Params().Len() != 0 || signature.Results().Len() != 1 {
		return false
	}

	return types.Identical(types.Unalias(signature.Results().At(0).Type()), types.Typ[types.String])
}
