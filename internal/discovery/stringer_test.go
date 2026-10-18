package discovery_test

import (
	"os"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
)

func TestStringerMethodSetsAreStructuralAndNeverExecuted(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := root.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	})

	for name, contents := range map[string]string{
		"go.mod": "module example.com/stringer\n\ngo 1.27\n",
		"app.go": stringerMethodSetFixture,
	} {
		err := root.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	var options discovery.Options

	options.Root, options.Offline = root.Name(), true

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{
		"example.com/stringer.Value": true, "*example.com/stringer.PointerOnly": true,
		"example.com/stringer.PointerOnly": false, "example.com/stringer.Printable": true,
		"example.com/stringer.Alias": true, "example.com/stringer.Promoted": true,
		"example.com/stringer.Ambiguous": false, "example.com/stringer.Wrong": false,
		"example.com/stringer.DefinedResult": false, "example.com/stringer.AliasResult": true,
		"example.com/stringer.Category": true, "example.com/stringer.Callback": true,
	}
	for _, typ := range code.Types {
		expected, exists := want[typ.Type]
		if !exists {
			continue
		}

		if typ.Stringer != expected {
			t.Fatalf("type %s Stringer=%t; want %t", typ.Type, typ.Stringer, expected)
		}

		delete(want, typ.Type)
	}

	if len(want) != 0 {
		t.Fatalf("missing fixture types: %+v", want)
	}
}

const stringerMethodSetFixture = `package app

type Value struct{}
func (Value) String() string { panic("discovery must not call String") }
type Other struct{}
func (Other) String() string { panic("discovery must not call String") }
type PointerOnly struct{}
func (*PointerOnly) String() string { panic("discovery must not call String") }
type Printable interface{ String() string }
type Alias = Value
type Promoted struct{ Value }
type Ambiguous struct{ Value; Other }
type Wrong struct{}
func (Wrong) String(int) string { panic("discovery must not call String") }
type Text string
type TextAlias = string
type DefinedResult struct{}
func (DefinedResult) String() Text { panic("discovery must not call String") }
type AliasResult struct{}
func (AliasResult) String() TextAlias { panic("discovery must not call String") }
type Category string
func (Category) String() string { panic("discovery must not call String") }
type Callback func()
func (Callback) String() string { panic("discovery must not call String") }
func Inspect(Value, PointerOnly, *PointerOnly, Printable, Alias, Promoted, Ambiguous, Wrong,
 DefinedResult, AliasResult, Category, Callback) {}
`
