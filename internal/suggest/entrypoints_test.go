package suggest_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/suggest"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestRankUsesNearestRegisteredHTTPEntrypoint(t *testing.T) {
	t.Parallel()

	main := function("example.com/app/cmd/api.main", "main")
	main.PackageName, main.ContextIndexes = "main", nil
	handler := function("example.com/app.Handle", "Handle")
	handler.ContextIndexes = nil
	handler.Entrypoints = []string{"net/http.HandleFunc", "net/http.(*ServeMux).HandleFunc", "net/http.HandleFunc"}
	bridge := function("example.com/app.Bridge", "Bridge")
	bridge.ContextIndexes = nil
	target := function("example.com/app.Run", "Run")
	code := new(model.CodeModel)
	code.Symbols = []model.Symbol{target, handler, bridge, main}
	code.CallGraph = &model.CallGraphInfo{
		Algorithm: "cha", Scope: "analyzed-package-callers", Conservative: true, Limitations: nil,
	}
	code.CallEdges = []model.CallRelation{
		{Caller: main.ID, Callee: bridge.ID, Precision: model.CallPrecisionStatic},
		{Caller: bridge.ID, Callee: target.ID, Precision: model.CallPrecisionStatic},
		{Caller: handler.ID, Callee: target.ID, Precision: model.CallPrecisionStatic},
	}
	snapshot := copyEntrypointModel(t, code)

	first := suggest.Rank(code)
	if len(first) != 1 || first[0].SymbolID != target.ID {
		t.Fatalf("technical handler became an instrumentation target: %+v", first)
	}

	evidence := strings.Join(first[0].Evidence, " ")
	if !strings.Contains(evidence, "example.com/app.Handle") || !strings.Contains(evidence, "net/http.HandleFunc") ||
		!strings.Contains(evidence, "through 1 static call(s)") {
		t.Fatalf("HTTP root provenance or nearest path missing: %+v", first)
	}

	if !reflect.DeepEqual(code, snapshot) {
		t.Fatal("ranking mutated caller model")
	}

	slices.Reverse(code.Symbols)
	slices.Reverse(code.CallEdges)

	if second := suggest.Rank(code); !reflect.DeepEqual(first, second) {
		t.Fatalf("entrypoint ranking depends on input order: %+v", second)
	}
}

func TestRankDoesNotPromoteConservativeHTTPCalls(t *testing.T) {
	t.Parallel()

	handler := function("example.com/app.Handle", "Handle")
	handler.ContextIndexes = nil
	handler.Entrypoints = []string{"net/http.HandleFunc"}
	target := function("example.com/app.Run", "Run")
	code := new(model.CodeModel)
	code.Symbols = []model.Symbol{handler, target}
	code.CallGraph = &model.CallGraphInfo{
		Algorithm: "cha", Scope: "analyzed-package-callers", Conservative: true, Limitations: nil,
	}
	code.CallEdges = []model.CallRelation{
		{Caller: handler.ID, Callee: target.ID, Precision: model.CallPrecisionConservative},
	}

	ranked := suggest.Rank(code)
	if len(ranked) != 1 || strings.Contains(strings.Join(ranked[0].Evidence, " "), "reachable") {
		t.Fatalf("conservative dispatch claimed HTTP reachability: %+v", ranked)
	}

	handler.Ownership = model.OwnershipDependency
	code.Symbols[0] = handler
	code.CallEdges[0].Precision = model.CallPrecisionStatic

	ranked = suggest.Rank(code)
	if len(ranked) != 1 || strings.Contains(strings.Join(ranked[0].Evidence, " "), "reachable") {
		t.Fatalf("dependency callback rooted application suggestions: %+v", ranked)
	}
}

func copyEntrypointModel(t *testing.T, code *model.CodeModel) *model.CodeModel {
	t.Helper()

	data, err := json.Marshal(code)
	if err != nil {
		t.Fatal(err)
	}

	snapshot := new(model.CodeModel)

	err = json.Unmarshal(data, snapshot)
	if err != nil {
		t.Fatal(err)
	}

	return snapshot
}

func TestRankTechnicalEntrypointTiesAreDeterministic(t *testing.T) {
	t.Parallel()

	left := function("example.com/app.A", "A")
	right := function("example.com/app.Z", "Z")
	left.ContextIndexes, right.ContextIndexes = nil, nil
	left.Entrypoints = []string{"net/http.HandleFunc"}
	right.Entrypoints = []string{"net/http.Handle"}
	target := function("example.com/app.Run", "Run")
	code := new(model.CodeModel)
	code.Symbols = []model.Symbol{right, target, left}
	code.CallGraph = &model.CallGraphInfo{
		Algorithm: "cha", Scope: "analyzed-package-callers", Conservative: true, Limitations: nil,
	}
	code.CallEdges = []model.CallRelation{
		{Caller: right.ID, Callee: target.ID, Precision: model.CallPrecisionStatic},
		{Caller: left.ID, Callee: target.ID, Precision: model.CallPrecisionStatic},
	}

	first := suggest.Rank(code)
	if len(first) != 1 || !strings.Contains(strings.Join(first[0].Evidence, " "), "example.com/app.A") {
		t.Fatalf("entrypoint tie did not use deterministic provenance: %+v", first)
	}

	for range 10 {
		slices.Reverse(code.Symbols)
		slices.Reverse(code.CallEdges)

		if next := suggest.Rank(code); !reflect.DeepEqual(first, next) {
			t.Fatalf("unstable HTTP root tie: %+v", next)
		}
	}
}

func TestRankExplainsRegisteredContextEntrypoints(t *testing.T) {
	t.Parallel()

	registered := function("example.com/app.(Endpoint).Run", "Run")
	registered.Entrypoints = []string{"google.golang.org/grpc.(*Server).RegisterService"}
	other := function("example.com/app.(Other).Run", "Run")
	code := new(model.CodeModel)
	code.Symbols = []model.Symbol{other, registered}

	ranked := suggest.Rank(code)
	if len(ranked) != 2 || ranked[0].SymbolID != registered.ID || ranked[0].Score <= ranked[1].Score {
		t.Fatalf("registered entrypoint received no ranking evidence: %+v", ranked)
	}

	evidence := strings.Join(ranked[0].Evidence, " ")
	if !strings.Contains(evidence, "registered entrypoint") ||
		!strings.Contains(evidence, "google.golang.org/grpc.(*Server).RegisterService") ||
		strings.Contains(evidence, "low signal") || strings.Contains(evidence, "reachable") {
		t.Fatalf("registered entrypoint evidence is misleading: %+v", ranked[0])
	}

	registered.ContextIndexes = nil
	code.Symbols[1] = registered

	if next := suggest.Rank(code); len(next) != 1 || next[0].SymbolID != other.ID {
		t.Fatalf("registration bypassed context eligibility: %+v", next)
	}
}
