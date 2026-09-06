package discovery

import (
	"go/types"
	"sort"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func (b *builder) collectInterfaceRelations(m *model.CodeModel) {
	type interfaceInfo struct {
		id        model.SymbolID
		pkg, name string
		typ       *types.Interface
	}
	var interfaces []interfaceInfo
	for _, p := range b.packages {
		for _, name := range p.Types.Scope().Names() {
			obj, ok := p.Types.Scope().Lookup(name).(*types.TypeName)
			if !ok {
				continue
			}
			iface, ok := obj.Type().Underlying().(*types.Interface)
			if !ok || !iface.IsMethodSet() {
				continue
			}
			interfaces = append(interfaces, interfaceInfo{id: model.SymbolID(p.PkgPath + "." + name), pkg: p.PkgPath, name: name, typ: iface.Complete()})
		}
	}
	sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].id < interfaces[j].id })
	for _, p := range b.packages {
		for _, name := range p.Types.Scope().Names() {
			obj, ok := p.Types.Scope().Lookup(name).(*types.TypeName)
			if !ok || obj.IsAlias() {
				continue
			}
			named, ok := obj.Type().(*types.Named)
			if !ok {
				continue
			}
			if _, ok := named.Underlying().(*types.Interface); ok {
				continue
			}
			for _, iface := range interfaces {
				for _, pointer := range []bool{false, true} {
					var concrete types.Type = named
					if pointer {
						concrete = types.NewPointer(named)
					}
					if !types.Implements(concrete, iface.typ) {
						continue
					}
					concreteID := model.SymbolID(p.PkgPath + "." + name)
					m.Implements = append(m.Implements, model.InterfaceRelation{InterfaceID: iface.id, InterfacePkg: iface.pkg, Interface: iface.name, ConcreteID: concreteID, Pointer: pointer})
					methods := types.NewMethodSet(concrete)
					for i := 0; i < iface.typ.NumMethods(); i++ {
						method := iface.typ.Method(i)
						selection := methods.Lookup(method.Pkg(), method.Name())
						if selection == nil {
							continue
						}
						fn, ok := selection.Obj().(*types.Func)
						if !ok {
							continue
						}
						signature := fn.Type().(*types.Signature)
						receiver := signature.Recv().Type()
						receiverPointer := false
						if ptr, ok := receiver.(*types.Pointer); ok {
							receiver = ptr.Elem()
							receiverPointer = true
						}
						receiverNamed, ok := receiver.(*types.Named)
						if !ok {
							continue
						}
						id := model.MethodID(fn.Pkg().Path(), model.Receiver{Type: receiverNamed.Obj().Name(), Pointer: receiverPointer}, fn.Name())
						m.InterfaceMethods = append(m.InterfaceMethods, model.InterfaceMethod{InterfaceID: iface.id, ConcreteID: concreteID, Pointer: pointer, SymbolID: id})
					}
				}
			}
		}
	}
}
