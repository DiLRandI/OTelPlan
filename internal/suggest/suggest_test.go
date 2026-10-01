package suggest_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/suggest"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestRankExplainsCallPathsAndBoundaries(t *testing.T) {
	t.Parallel()

	main := function("example.com/shop/cmd/api.main", "main")
	main.PackageName, main.ContextIndexes = "main", nil
	checkout := function("example.com/shop/checkout.Run", "Run")
	checkout.ErrorIndexes = []int{0}
	port := function("example.com/shop/payment.Charge", "Charge")
	port.ErrorIndexes = []int{0}
	utility := function("example.com/shop/common.Service", "Service")

	code := new(model.CodeModel)
	code.Symbols = []model.Symbol{utility, port, main, checkout}
	code.CallGraph = &model.CallGraphInfo{
		Algorithm: "cha", Scope: "analyzed-package-callers", Conservative: true, Limitations: nil,
	}
	method := new(model.InterfaceMethod)
	method.SymbolID = port.ID
	code.InterfaceMethods = []model.InterfaceMethod{*method}
	code.CallEdges = []model.CallRelation{
		{Caller: main.ID, Callee: checkout.ID, Precision: model.CallPrecisionStatic},
		{Caller: checkout.ID, Callee: port.ID, Precision: model.CallPrecisionStatic},
		{Caller: port.ID, Callee: "database/sql.(*DB).ExecContext", Precision: model.CallPrecisionStatic},
	}

	first := suggest.Rank(code)
	if len(first) != 3 || first[0].SymbolID != port.ID ||
		first[1].SymbolID != checkout.ID || first[2].SymbolID != utility.ID {
		t.Fatalf("unexpected suggestion order: %+v", first)
	}

	if first[0].Confidence != "high" || first[2].Confidence != "low" {
		t.Fatalf("unexpected confidence: %+v", first)
	}

	for _, evidence := range []string{"interface method", "infrastructure API", "reachable from main.main"} {
		if !strings.Contains(strings.Join(first[0].Evidence, " "), evidence) {
			t.Fatalf("missing evidence %q: %+v", evidence, first[0])
		}
	}

	reordered := *code
	reordered.Symbols = slices.Clone(code.Symbols)
	slices.Reverse(reordered.Symbols)
	reordered.CallEdges = slices.Clone(code.CallEdges)
	slices.Reverse(reordered.CallEdges)
	reordered.CallEdges = append(reordered.CallEdges, reordered.CallEdges...)

	if second := suggest.Rank(&reordered); !reflect.DeepEqual(first, second) {
		t.Fatalf("ranking changed with input order or duplicate edges: %+v", second)
	}
}

func TestRankDoesNotTreatConservativeCallsAsReachable(t *testing.T) {
	t.Parallel()

	main := function("example.com/shop/cmd/api.main", "main")
	main.PackageName, main.ContextIndexes = "main", nil
	target := function("example.com/shop/checkout.Run", "Run")
	code := new(model.CodeModel)
	code.Symbols = []model.Symbol{main, target}
	code.CallGraph = &model.CallGraphInfo{
		Algorithm: "cha", Scope: "analyzed-package-callers", Conservative: true, Limitations: nil,
	}
	code.CallEdges = []model.CallRelation{{Caller: main.ID, Callee: target.ID, Precision: model.CallPrecisionConservative}}

	suggestions := suggest.Rank(code)
	if len(suggestions) != 1 || suggestions[0].Confidence != "low" {
		t.Fatalf("conservative call became a high-confidence target: %+v", suggestions)
	}

	evidence := strings.Join(suggestions[0].Evidence, " ")
	if !strings.Contains(evidence, "conservative") || strings.Contains(evidence, "reachable") {
		t.Fatalf("conservative call was presented as proven reachability: %s", evidence)
	}
}

func TestRankKeepsConservativePolicyCandidates(t *testing.T) {
	t.Parallel()

	service := function("example.com/shop/Service", "Service")
	repository := function("example.com/shop/Repository", "Repository")
	withoutContext := function("example.com/shop/NoContext", "NoContext")
	withoutContext.ContextIndexes = nil
	dependency := function("example.com/dependency.Run", "Run")
	dependency.Ownership = model.OwnershipDependency
	generated := function("example.com/shop.Generated", "Generated")
	generated.Generated = true
	testFile := function("example.com/shop.TestOnly", "TestOnly")
	testFile.TestFile = true
	variadic := function("example.com/shop.Variadic", "Variadic")
	variadic.Variadic = true
	generic := function("example.com/shop.Generic", "Generic")
	generic.Generics = &model.GenericInfo{TypeParams: []string{"T"}}
	withoutBody := function("example.com/shop.NoBody", "NoBody")
	withoutBody.HasBody = false

	code := new(model.CodeModel)
	code.Symbols = []model.Symbol{
		service, repository, withoutContext, dependency, generated, testFile, variadic, generic, withoutBody,
	}

	suggestions := suggest.Rank(code)
	if len(suggestions) != 2 || suggestions[0].Score != suggestions[1].Score {
		t.Fatalf("names affected ranking or unsupported symbols were suggested: %+v", suggestions)
	}

	if suggestions[0].SymbolID != repository.ID || suggestions[1].SymbolID != service.ID {
		t.Fatalf("equal-scored suggestions were not ordered by canonical symbol: %+v", suggestions)
	}

	if !reflect.DeepEqual(suggestions, suggest.Rank(code)) {
		t.Fatal("repeated analysis changed suggestions")
	}
}

func function(id model.SymbolID, name string) model.Symbol {
	symbol := new(model.Symbol)
	symbol.ID, symbol.Kind, symbol.Name, symbol.PackageName = id, model.SymbolFunction, name, "shop"
	symbol.Visibility, symbol.Ownership = model.VisibilityExported, model.OwnershipApplication
	symbol.ContextIndexes, symbol.HasBody = []int{0}, true

	return *symbol
}
