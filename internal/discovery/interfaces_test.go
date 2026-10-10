package discovery_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestInterfaceRelationsUseCanonicalMethodSets(t *testing.T) {
	t.Parallel()

	root := interfaceFixture(t)

	var options discovery.Options

	options.Root, options.Offline = root.Name(), true
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	first, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	second, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	sameRelations := reflect.DeepEqual(first.Implements, second.Implements)

	sameMethods := reflect.DeepEqual(first.InterfaceMethods, second.InterfaceMethods)
	if !sameRelations || !sameMethods {
		t.Fatal("interface indexes changed between identical loads")
	}

	assertNamedImplementors(t, first)
	assertMethodBindings(t, first)
	assertPrivateAndConstraintInterfaces(t, first)
}

func assertNamedImplementors(t *testing.T, code *model.CodeModel) {
	t.Helper()

	implementors := map[string]bool{}
	for _, rel := range code.Implementors("example.com/shop/ports", "Operation") {
		implementors[string(rel.ConcreteID)+":"+strconv.FormatBool(rel.Pointer)] = true
	}

	for _, want := range []string{
		"example.com/shop.Value:false",
		"example.com/shop.Pointer:true",
		"example.com/shop.Embedded:false",
		"example.com/shop.Embedded:true",
	} {
		if !implementors[want] {
			t.Errorf("missing implementor %s; got %v", want, implementors)
		}
	}

	if !implementors["example.com/shop.Value:true"] || implementors["example.com/shop.Pointer:false"] {
		t.Fatalf("incorrect value/pointer method-set relation: %v", implementors)
	}

	if implementors["example.com/shop.ValueAlias:false"] || implementors["example.com/shop.ValueAlias:true"] {
		t.Fatalf("alias indexed as concrete implementor: %v", implementors)
	}
}

func assertMethodBindings(t *testing.T, code *model.CodeModel) {
	t.Helper()

	methodIDs := map[string]bool{}
	promoted := false

	for _, binding := range code.InterfaceMethods {
		if binding.InterfaceID == "example.com/shop/ports.Operation" {
			methodIDs[string(binding.SymbolID)] = true

			if binding.ConcreteID == "example.com/shop.Embedded" {
				promoted = true

				if binding.SymbolID != "example.com/shop.(Base).Run" {
					t.Errorf("promoted method resolved to %s, want original Base.Run", binding.SymbolID)
				}
			}
		}
	}

	if !promoted || !methodIDs["example.com/shop.(Base).Run"] || methodIDs["example.com/shop.(Base).Extra"] {
		t.Fatal("promoted method binding for Embedded is missing")
	}

	assertInterfaceAliasMethod(t, code)
}

func assertInterfaceAliasMethod(t *testing.T, code *model.CodeModel) {
	t.Helper()

	aliasMethod := false

	for _, binding := range code.InterfaceMethods {
		matchesAlias := binding.InterfaceID == "example.com/shop/ports.OperationAlias"

		matchesMethod := binding.SymbolID == "example.com/shop.(Base).Run"

		if matchesAlias && matchesMethod {
			aliasMethod = true
		}
	}

	if !aliasMethod {
		t.Fatal("method-set interface alias was not indexed")
	}
}

func assertPrivateAndConstraintInterfaces(t *testing.T, code *model.CodeModel) {
	t.Helper()

	if got := code.Implementors("example.com/shop/ports", "hidden"); len(got) == 0 {
		t.Fatal("private interface membership was not indexed")
	}

	if got := len(code.Implementors("example.com/shop", "Constraint")); got != 0 {
		t.Fatalf("type-set constraint implementors = %d, want 0", got)
	}
}

func interfaceFixture(t *testing.T) *os.Root {
	t.Helper()

	dir := t.TempDir()

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := root.Close()
		if err != nil {
			t.Errorf("close interface fixture root: %v", err)
		}
	})

	files := map[string]string{
		"go.mod": "module example.com/shop\n\ngo 1.27\n",
		"ports/port.go": `package ports
type Operation interface { Run() }
type OperationAlias = Operation
type hidden interface { Extra() }
`,
		"interfaces.go": `package shop
import "example.com/shop/ports"
type Base struct{}
func (Base) Run() {}
func (Base) Extra() {}
type Embedded struct { Base }
func (*Embedded) Other() {}
type Value struct{}
func (Value) Run() {}
type Pointer struct{}
func (*Pointer) Run() {}
type ValueAlias = Value
type Constraint interface { ~int }
var _ ports.OperationAlias
`,
	}

	for name, contents := range files {
		err := root.MkdirAll(filepath.Dir(name), 0o700)
		if err != nil {
			t.Fatal(err)
		}

		err = root.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	return root
}
