package resolve

import (
	"github.com/DiLRandI/OTelPlan/pkg/model"
	"testing"
)

func TestGlob(t *testing.T) {
	for _, tc := range []struct {
		pattern, value string
		want           bool
	}{
		{"**/*.go", "root.go", true}, {"**/*.go", "a/b/root.go", true},
		{"a/**", "a", true}, {"a/*", "a/b/c", false}, {"a/**/b", "a/b", true},
		{"[AB]?", "Ax", true}, {"Run", "RunMore", false}, {"**/mock_*.go", "mock_x.go", true},
	} {
		t.Run(tc.pattern+tc.value, func(t *testing.T) {
			got, err := Glob(tc.pattern, tc.value)
			if err != nil || got != tc.want {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
	if _, err := Glob("[", "a"); err == nil {
		t.Fatal("malformed pattern accepted")
	}
}

func TestMatchSelectors(t *testing.T) {
	s := model.Symbol{ID: "example.com/x.(*Worker).Run", Kind: model.SymbolMethod, Name: "Run", PackageImportPath: "example.com/x", Location: model.SourceLocation{File: "domain/work.go"}, Receiver: &model.Receiver{Type: "Worker", Pointer: true}, Visibility: model.VisibilityExported, Ownership: model.OwnershipApplication, ContextIndexes: []int{0}, ErrorIndexes: []int{0}}
	m := &model.CodeModel{InterfaceMethods: []model.InterfaceMethod{{InterfaceID: "example.com/ports.Operation", SymbolID: s.ID}}}
	yes, no := true, false
	for _, tc := range []struct {
		name     string
		selector model.Match
		want     bool
	}{
		{"package", model.Match{Packages: []string{"wrong", "example.com/*"}}, true},
		{"file", model.Match{Files: []string{"**/*.go"}}, true},
		{"exact", model.Match{Symbols: []string{string(s.ID)}}, true},
		{"exact is not glob", model.Match{Symbols: []string{"example.com/**"}}, false},
		{"function excludes method", model.Match{Functions: []string{"*"}}, false},
		{"method", model.Match{Methods: []string{"Run"}}, true},
		{"receiver", model.Match{Receivers: []string{"Work*"}}, true},
		{"interface", model.Match{Implements: []string{"example.com/ports.Operation"}}, true},
		{"unrelated interface", model.Match{Implements: []string{"example.com/ports.Other"}}, false},
		{"exported", model.Match{Exported: &yes}, true},
		{"context", model.Match{HasContext: &yes}, true},
		{"error", model.Match{ReturnsError: &yes}, true},
		{"ownership", model.Match{Ownership: model.OwnershipDependency}, false},
		{"and", model.Match{Methods: []string{"Run"}, HasContext: &no}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Matches(m, s, tc.selector)
			if err != nil || got != tc.want {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
}
