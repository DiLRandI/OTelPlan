package resolve_test

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/resolve"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestGlob(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		pattern, value string
		want           bool
	}{
		{pattern: "**/*.go", value: "root.go", want: true},
		{pattern: "**/*.go", value: "a/b/root.go", want: true},
		{pattern: "a/**", value: "a", want: true},
		{pattern: "a/*", value: "a/b/c", want: false},
		{pattern: "a/**/b", value: "a/b", want: true},
		{pattern: "[AB]?", value: "Ax", want: true},
		{pattern: "Run", value: "RunMore", want: false},
		{pattern: "**/mock_*.go", value: "mock_x.go", want: true},
	} {
		t.Run(testCase.pattern+testCase.value, func(t *testing.T) {
			t.Parallel()

			got, err := resolve.Glob(testCase.pattern, testCase.value)
			if err != nil || got != testCase.want {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}

	_, err := resolve.Glob("[", "a")
	if err == nil {
		t.Fatal("malformed pattern accepted")
	}
}

func TestMatchSelectors(t *testing.T) {
	t.Parallel()

	symbol := model.Symbol{
		ID:                "example.com/x.(*Worker).Run",
		Kind:              model.SymbolMethod,
		PackageImportPath: "example.com/x",
		PackageName:       "",
		Name:              "Run",
		Receiver:          &model.Receiver{Name: "", Type: "Worker", Pointer: true},
		Location:          model.SourceLocation{File: "domain/work.go", Line: 0, Column: 0},
		Visibility:        model.VisibilityExported,
		Parameters:        nil,
		Results:           nil,
		ContextIndexes:    []int{0},
		ErrorIndexes:      []int{0},
		Generics:          nil,
		Generated:         false,
		TestFile:          false,
		Ownership:         model.OwnershipApplication,
		Signature:         "",
		HasBody:           false,
		Variadic:          false,
	}

	var buildEnvironment model.BuildEnvironment

	codeModel := &model.CodeModel{
		GoVersion:     "",
		ModuleRoot:    "",
		WorkspaceFile: "",
		Modules:       nil,
		Packages:      nil,
		Symbols:       nil,
		Types:         nil,
		Implements:    nil,
		InterfaceMethods: []model.InterfaceMethod{{
			InterfaceID: "example.com/ports.Operation", ConcreteID: "", Pointer: false, SymbolID: symbol.ID,
		}},
		CallGraph:      nil,
		CallEdges:      nil,
		BuildTags:      nil,
		GOOS:           "",
		GOARCH:         "",
		EffectiveBuild: buildEnvironment,
	}

	yes, noContext := true, false
	for _, testCase := range []struct {
		name     string
		selector model.Match
		want     bool
	}{
		{
			name: "package",
			selector: model.Match{
				Packages: []string{"wrong", "example.com/*"}, Files: nil, Symbols: nil, Functions: nil,
				Receivers: nil, Methods: nil, Implements: nil, Exported: nil, HasContext: nil, ReturnsError: nil,
				Ownership: "",
			},
			want: true,
		},
		{
			name: "file",
			selector: model.Match{
				Packages: nil, Files: []string{"**/*.go"}, Symbols: nil, Functions: nil, Receivers: nil,
				Methods: nil, Implements: nil, Exported: nil, HasContext: nil, ReturnsError: nil, Ownership: "",
			},
			want: true,
		},
		{
			name: "exact",
			selector: model.Match{
				Packages: nil, Files: nil, Symbols: []string{string(symbol.ID)}, Functions: nil, Receivers: nil,
				Methods: nil, Implements: nil, Exported: nil, HasContext: nil, ReturnsError: nil, Ownership: "",
			},
			want: true,
		},
		{
			name: "exact is not glob",
			selector: model.Match{
				Packages: nil, Files: nil, Symbols: []string{"example.com/**"}, Functions: nil, Receivers: nil,
				Methods: nil, Implements: nil, Exported: nil, HasContext: nil, ReturnsError: nil, Ownership: "",
			},
			want: false,
		},
		{
			name: "function excludes method",
			selector: model.Match{
				Packages: nil, Files: nil, Symbols: nil, Functions: []string{"*"}, Receivers: nil, Methods: nil,
				Implements: nil, Exported: nil, HasContext: nil, ReturnsError: nil, Ownership: "",
			},
			want: false,
		},
		{
			name: "method",
			selector: model.Match{
				Packages: nil, Files: nil, Symbols: nil, Functions: nil, Receivers: nil, Methods: []string{"Run"},
				Implements: nil, Exported: nil, HasContext: nil, ReturnsError: nil, Ownership: "",
			},
			want: true,
		},
		{
			name: "receiver",
			selector: model.Match{
				Packages: nil, Files: nil, Symbols: nil, Functions: nil, Receivers: []string{"Work*"}, Methods: nil,
				Implements: nil, Exported: nil, HasContext: nil, ReturnsError: nil, Ownership: "",
			},
			want: true,
		},
		{
			name: "interface",
			selector: model.Match{
				Packages: nil, Files: nil, Symbols: nil, Functions: nil, Receivers: nil, Methods: nil,
				Implements: []string{"example.com/ports.Operation"}, Exported: nil, HasContext: nil, ReturnsError: nil,
				Ownership: "",
			},
			want: true,
		},
		{
			name: "unrelated interface",
			selector: model.Match{
				Packages: nil, Files: nil, Symbols: nil, Functions: nil, Receivers: nil, Methods: nil,
				Implements: []string{"example.com/ports.Other"}, Exported: nil, HasContext: nil, ReturnsError: nil,
				Ownership: "",
			},
			want: false,
		},
		{
			name: "exported",
			selector: model.Match{
				Packages: nil, Files: nil, Symbols: nil, Functions: nil, Receivers: nil, Methods: nil,
				Implements: nil, Exported: &yes, HasContext: nil, ReturnsError: nil, Ownership: "",
			},
			want: true,
		},
		{
			name: "context",
			selector: model.Match{
				Packages: nil, Files: nil, Symbols: nil, Functions: nil, Receivers: nil, Methods: nil,
				Implements: nil, Exported: nil, HasContext: &yes, ReturnsError: nil, Ownership: "",
			},
			want: true,
		},
		{
			name: "error",
			selector: model.Match{
				Packages: nil, Files: nil, Symbols: nil, Functions: nil, Receivers: nil, Methods: nil,
				Implements: nil, Exported: nil, HasContext: nil, ReturnsError: &yes, Ownership: "",
			},
			want: true,
		},
		{
			name: "ownership",
			selector: model.Match{
				Packages: nil, Files: nil, Symbols: nil, Functions: nil, Receivers: nil, Methods: nil,
				Implements: nil, Exported: nil, HasContext: nil, ReturnsError: nil,
				Ownership: model.OwnershipDependency,
			},
			want: false,
		},
		{
			name: "and",
			selector: model.Match{
				Packages: nil, Files: nil, Symbols: nil, Functions: nil, Receivers: nil, Methods: []string{"Run"},
				Implements: nil, Exported: nil, HasContext: &noContext, ReturnsError: nil, Ownership: "",
			},
			want: false,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolve.Matches(codeModel, symbol, testCase.selector)
			if err != nil || got != testCase.want {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
}
