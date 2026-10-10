package discovery

import (
	"go/types"
	"sort"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

type interfaceInfo struct {
	id        model.SymbolID
	pkg, name string
	typ       *types.Interface
}

func (indexer *builder) collectInterfaceRelations(code *model.CodeModel) {
	interfaces := indexer.methodSetInterfaces()

	for _, pkg := range indexer.packages {
		for _, name := range pkg.Types.Scope().Names() {
			named := concreteNamedType(pkg.Types.Scope().Lookup(name))
			if named == nil {
				continue
			}

			concreteID := model.SymbolID(pkg.PkgPath + "." + name)
			collectConcreteInterfaceRelations(code, concreteID, named, interfaces)
		}
	}
}

func (indexer *builder) methodSetInterfaces() []interfaceInfo {
	var interfaces []interfaceInfo

	for _, pkg := range indexer.packages {
		for _, name := range pkg.Types.Scope().Names() {
			obj, isTypeName := pkg.Types.Scope().Lookup(name).(*types.TypeName)
			if !isTypeName {
				continue
			}

			iface, isInterface := obj.Type().Underlying().(*types.Interface)
			if !isInterface || !iface.IsMethodSet() {
				continue
			}

			interfaces = append(interfaces, interfaceInfo{
				id: model.SymbolID(pkg.PkgPath + "." + name), pkg: pkg.PkgPath, name: name, typ: iface.Complete(),
			})
		}
	}

	sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].id < interfaces[j].id })

	return interfaces
}

func concreteNamedType(object types.Object) *types.Named {
	name, isTypeName := object.(*types.TypeName)
	if !isTypeName || name.IsAlias() {
		return nil
	}

	named, isNamed := name.Type().(*types.Named)
	if !isNamed {
		return nil
	}

	_, isInterface := named.Underlying().(*types.Interface)
	if isInterface {
		return nil
	}

	return named
}

func collectConcreteInterfaceRelations(code *model.CodeModel, concreteID model.SymbolID,
	named *types.Named, interfaces []interfaceInfo) {
	for _, iface := range interfaces {
		for _, pointer := range []bool{false, true} {
			var concrete types.Type = named

			if pointer {
				concrete = types.NewPointer(named)
			}

			if !types.Implements(concrete, iface.typ) {
				continue
			}

			code.Implements = append(code.Implements, model.InterfaceRelation{
				InterfaceID: iface.id, InterfacePkg: iface.pkg, Interface: iface.name,
				ConcreteID: concreteID, Pointer: pointer,
			})
			collectInterfaceMethodBindings(code, concreteID, concrete, pointer, iface)
		}
	}
}

func collectInterfaceMethodBindings(code *model.CodeModel, concreteID model.SymbolID, concrete types.Type,
	pointer bool, iface interfaceInfo) {
	methods := types.NewMethodSet(concrete)

	for method := range iface.typ.Methods() {
		selection := methods.Lookup(method.Pkg(), method.Name())
		if selection == nil {
			continue
		}

		function, isFunction := selection.Obj().(*types.Func)
		if !isFunction {
			continue
		}

		symbolID, known := interfaceDeclaringMethodID(function)
		if !known {
			continue
		}

		code.InterfaceMethods = append(code.InterfaceMethods, model.InterfaceMethod{
			InterfaceID: iface.id, ConcreteID: concreteID, Pointer: pointer, SymbolID: symbolID,
		})
	}
}

func interfaceDeclaringMethodID(function *types.Func) (model.SymbolID, bool) {
	receiver := function.Signature().Recv().Type()
	receiverPointer := false

	if pointer, isPointer := receiver.(*types.Pointer); isPointer {
		receiver = pointer.Elem()
		receiverPointer = true
	}

	receiverNamed, isNamed := receiver.(*types.Named)
	if !isNamed {
		return "", false
	}

	var identity model.Receiver

	identity.Type, identity.Pointer = receiverNamed.Obj().Name(), receiverPointer

	return model.MethodID(function.Pkg().Path(), identity, function.Name()), true
}
