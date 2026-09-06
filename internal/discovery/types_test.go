package discovery

import "testing"

func TestAttributeTypeGraph(t *testing.T) {
	root := fixture(t, map[string]string{"values.go": `package shop
type Kind string
type Details struct { Kind Kind; password string }
type Request struct { *Details; Next *Request; Count int; Payload []byte }
func Execute(request *Request) (result Details) { return Details{} }
`})
	code, err := Load(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	symbol, ok := code.Symbol("example.com/shop.Execute")
	if !ok || symbol.Parameters[0].Type != "*example.com/shop.Request" {
		t.Fatalf("ambiguous parameter type: %+v", symbol)
	}
	foundRequest, foundKind := false, false
	seen := map[string]bool{}
	for _, typ := range code.Types {
		if seen[typ.Type] {
			t.Fatalf("duplicate type %s", typ.Type)
		}
		seen[typ.Type] = true
		if typ.Type == "example.com/shop.Request" {
			foundRequest = true
			if typ.Kind != "struct" || len(typ.Fields) != 4 || !typ.Fields[0].Embedded {
				t.Fatalf("invalid struct shape: %+v", typ)
			}
		}
		if typ.Type == "example.com/shop.Kind" {
			foundKind = true
			if typ.Kind != "string" {
				t.Fatalf("named primitive: %+v", typ)
			}
		}
		if typ.Type == "example.com/shop.Details" && typ.Fields[1].Exported {
			t.Fatal("private field marked exported")
		}
	}
	if !foundRequest || !foundKind {
		t.Fatalf("missing types: %+v", code.Types)
	}
}
