package discovery

import "testing"

func TestInterfaceMethodsExcludeUnrelatedAndResolvePromoted(t *testing.T) {
	root := fixture(t, map[string]string{
		"ports/port.go": "package ports\ntype Operation interface { Run() }\ntype hidden interface { Extra() }\n",
		"worker.go":     "package shop\ntype Base struct{}\nfunc (Base) Run() {}\nfunc (Base) Extra() {}\ntype Embedded struct { Base }\nfunc (*Embedded) Other() {}\n",
	})
	m, err := Load(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, binding := range m.InterfaceMethods {
		if binding.InterfaceID != "example.com/shop/ports.Operation" {
			continue
		}
		if binding.SymbolID != "example.com/shop.(Base).Run" {
			t.Fatalf("unrelated or nonexistent method: %+v", binding)
		}
		if binding.ConcreteID == "example.com/shop.Embedded" {
			found = true
		}
	}
	if !found {
		t.Fatal("promoted interface method not indexed")
	}
	if len(m.Implementors("example.com/shop/ports", "hidden")) == 0 {
		t.Fatal("private interface not indexed")
	}
}
