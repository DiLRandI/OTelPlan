package otelc

import (
	"fmt"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func (renderer *accessorRenderer) renderErrorTypeAccessor(binding AccessorBinding) error {
	ownAliases, err := accessorTypeImports([]model.TypeImport{
		{Alias: "otelplanreflect", Path: "reflect"}, {Alias: "otelplanstrings", Path: "strings"},
	}, renderer.symbol.PackageImportPath, renderer.imports)
	if err != nil {
		return err
	}

	reflectPrefix, stringsPrefix := "otelplanreflect.", "otelplanstrings."
	if ownAliases["otelplanreflect"] {
		reflectPrefix = ""
	}

	if ownAliases["otelplanstrings"] {
		stringsPrefix = ""
	}

	fmt.Fprintf(&renderer.body, "func %s(value any) (string, bool) {\n"+
		"typed, ok := value.(error)\nif !ok || typed == nil { return \"\", false }\n", binding.Function)
	// Value formatting can invoke methods; full type formatting can expose anonymous type tags.
	fmt.Fprintf(&renderer.body, "typ := %sTypeOf(typed)\n"+
		"for typ.Kind() == %sPointer { typ = typ.Elem() }\n"+
		"name, _, _ := %sCut(typ.Name(), \"[\")\n"+
		"if name == \"\" { return \"unnamed\", true }\n"+
		"if path := typ.PkgPath(); path != \"\" { return path + \".\" + name, true }\n"+
		"return name, true\n}\n", reflectPrefix, reflectPrefix, stringsPrefix)

	return nil
}
