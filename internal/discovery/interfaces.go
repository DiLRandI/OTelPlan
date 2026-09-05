package discovery

import (
	"go/types"
	"sort"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/tools/go/packages"
)

func (b *builder) collectInterfaceRelations(m *model.CodeModel) {
	type ifaceKey struct {
		pkg  string
		name string
	}
	var ifaces []ifaceKey
	for _, p := range b.packages {
		scope := p.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			tn, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}
			if _, ok := tn.Type().Underlying().(*types.Interface); ok {
				ifaces = append(ifaces, ifaceKey{pkg: p.PkgPath, name: name})
			}
		}
	}
	sort.Slice(ifaces, func(i, j int) bool {
		if ifaces[i].pkg != ifaces[j].pkg {
			return ifaces[i].pkg < ifaces[j].pkg
		}
		return ifaces[i].name < ifaces[j].name
	})

	seen := map[model.InterfaceRelation]bool{}
	for _, p := range b.packages {
		if p.Module == nil || !p.Module.Main {
			continue
		}
		scope := p.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			tn, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}
			named, ok := tn.Type().(*types.Named)
			if !ok {
				continue
			}
			if _, isIface := named.Underlying().(*types.Interface); isIface {
				continue
			}
			for _, iface := range ifaces {
				if iface.pkg == p.PkgPath && iface.name == name {
					continue
				}
				var ifaceType types.Type
				for _, q := range b.packages {
					if q.PkgPath == iface.pkg {
						iscope := q.Types.Scope()
						iObj := iscope.Lookup(iface.name)
						if iObj != nil {
							ifaceType = iObj.Type()
						}
						break
					}
				}
				if ifaceType == nil {
					continue
				}
				if types.Implements(named, ifaceTypeUnderlying(ifaceType)) {
					b.addRelation(m, seen, p, named, iface, false)
				}
				pointer := types.NewPointer(named)
				if types.Implements(pointer, ifaceTypeUnderlying(ifaceType)) {
					b.addRelation(m, seen, p, named, iface, true)
				}
			}
		}
	}
}

func ifaceTypeUnderlying(t types.Type) *types.Interface {
	if iface, ok := t.Underlying().(*types.Interface); ok {
		return iface
	}
	return nil
}

func (b *builder) addRelation(m *model.CodeModel, seen map[model.InterfaceRelation]bool, p *packages.Package, named *types.Named, iface struct{ pkg, name string }, pointer bool) {
	rel := model.InterfaceRelation{
		InterfaceID:  model.SymbolID(iface.pkg + "." + iface.name),
		InterfacePkg: iface.pkg,
		Interface:    iface.name,
		ConcreteID:   model.SymbolID(p.PkgPath + "." + named.Obj().Name()),
		Pointer:      pointer,
	}
	if seen[rel] {
		return
	}
	seen[rel] = true
	m.Implements = append(m.Implements, rel)
}
