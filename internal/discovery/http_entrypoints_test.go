package discovery_test

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestDiscoveryIndexesHTTPEntrypoints(t *testing.T) {
	t.Parallel()

	root := httpEntrypointFixture(t)
	first := loadHTTPEntrypointModel(t, root)
	second := loadHTTPEntrypointModel(t, root)

	want := map[model.SymbolID][]string{
		"example.com/httpentrypoints.HandlerFuncTarget": {
			"net/http.(*ServeMux).Handle",
			"net/http.(*ServeMux).HandleFunc",
			"net/http.Handle",
			"net/http.HandleFunc",
		},
		"example.com/httpentrypoints.HandlerTarget": {
			"net/http.(*ServeMux).Handle",
			"net/http.Handle",
		},
		"example.com/httpentrypoints.(ConcreteHandler).ServeHTTP": {
			"net/http.(*ServeMux).Handle",
			"net/http.Handle",
		},
		"example.com/httpentrypoints.(BoundHandler).Handle": {
			"net/http.(*ServeMux).Handle",
			"net/http.(*ServeMux).HandleFunc",
		},
		"example.com/httpentrypoints.(*EmbeddedHandler).ServeHTTP": {"net/http.Handle"},
		"example.com/httpentrypoints.(Custom).ServeHTTP": {
			"net/http.Handle",
		},
	}

	for symbolID, entrypoints := range want {
		assertHTTPEntrypoints(t, first, symbolID, entrypoints)
		assertHTTPEntrypoints(t, second, symbolID, entrypoints)
	}

	for _, symbolID := range []model.SymbolID{
		"example.com/httpentrypoints.UnregisteredHandler",
		"example.com/httpentrypoints.FuncVarTarget",
		"example.com/httpentrypoints.NeverInvoked",
		"example.com/httpentrypoints.(unknownHandler).ServeHTTP",
		"example.com/httpentrypoints.FakeUnregisteredHandler",
	} {
		assertHTTPEntrypoints(t, first, symbolID, nil)
	}

	if !reflect.DeepEqual(first.Symbols, second.Symbols) {
		t.Fatal("HTTP entrypoint metadata was not deterministic")
	}

	encoded, err := json.Marshal(first.Symbols)
	if err != nil {
		t.Fatal(err)
	}

	for _, route := range []string{
		"/private-route-global",
		"/private-route-mux",
		"/private-route-method-expression",
		"/private-route-alias",
	} {
		if strings.Contains(string(encoded), route) {
			t.Fatalf("HTTP route value leaked into symbol metadata: %s", route)
		}
	}
}

func assertHTTPEntrypoints(t *testing.T, code *model.CodeModel, symbolID model.SymbolID, want []string) {
	t.Helper()

	symbol, found := code.Symbol(symbolID)
	if !found {
		t.Fatalf("missing HTTP fixture symbol %s", symbolID)
	}

	if !slices.Equal(symbol.Entrypoints, want) {
		t.Fatalf("HTTP entrypoints for %s = %v, want %v", symbolID, symbol.Entrypoints, want)
	}
}

func loadHTTPEntrypointModel(t *testing.T, root string) *model.CodeModel {
	t.Helper()

	var options discovery.Options

	options.Root, options.Offline, options.IncludeTests = root, true, true
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	return code
}

func httpEntrypointFixture(t *testing.T) string {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := root.Close()
		if closeErr != nil {
			t.Errorf("close HTTP entrypoint fixture: %v", closeErr)
		}
	})

	files := map[string]string{
		"go.mod": "module example.com/httpentrypoints\n\ngo 1.27\n",
		"handlers.go": `package httpentrypoints

import (
	"net/http"
	aliashttp "net/http"
)

func HandlerFuncTarget(http.ResponseWriter, *http.Request) {}
func HandlerTarget(http.ResponseWriter, *http.Request) {}

type ConcreteHandler struct{}

func (ConcreteHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

type EmbeddedHandler struct{}
func (*EmbeddedHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}
type PromotedHandler struct{ *EmbeddedHandler }

type BoundHandler struct{}

func (BoundHandler) Handle(http.ResponseWriter, *http.Request) {}

type Custom func(http.ResponseWriter, *http.Request)

func (Custom) ServeHTTP(http.ResponseWriter, *http.Request) {}

func NeverInvoked(http.ResponseWriter, *http.Request) {}

func UnregisteredHandler(http.ResponseWriter, *http.Request) {}
func FuncVarTarget(http.ResponseWriter, *http.Request) {}
var functionVariable = FuncVarTarget

type unknownHandler struct{}

func (unknownHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

var interfaceVariable http.Handler = unknownHandler{}

func FakeUnregisteredHandler(http.ResponseWriter, *http.Request) {}

type fakeMux struct{}

func (fakeMux) HandleFunc(string, http.HandlerFunc) {}
func (fakeMux) Handle(string, http.Handler) {}

func register() {
	http.HandleFunc("/private-route-global", HandlerFuncTarget)
	http.HandleFunc("/private-route-global-duplicate", HandlerFuncTarget)
	http.Handle("/private-route-global-handler-func-conversion", http.HandlerFunc(HandlerFuncTarget))
	http.Handle("/private-route-global-handler", http.HandlerFunc(HandlerTarget))
	http.Handle("/private-route-concrete", ConcreteHandler{})
 http.Handle("/private-promoted", PromotedHandler{EmbeddedHandler: &EmbeddedHandler{}})
	http.Handle("/private-route-interface-conversion", http.Handler(ConcreteHandler{}))
	http.Handle("/private-route-custom", Custom(NeverInvoked))

	mux := http.NewServeMux()
	mux.HandleFunc("/private-route-mux", HandlerFuncTarget)
	mux.Handle("/private-route-mux-handler-func-conversion", http.HandlerFunc(HandlerFuncTarget))
	mux.Handle("/private-route-mux-handler", http.HandlerFunc(HandlerTarget))
	mux.Handle("/private-route-mux-concrete", ConcreteHandler{})
	mux.HandleFunc("/private-route-bound", BoundHandler{}.Handle)
	mux.Handle("/private-route-bound-handler", http.HandlerFunc(BoundHandler{}.Handle))

	aliashttp.HandleFunc("/private-route-alias", HandlerFuncTarget)
	aliashttp.Handle("/private-route-alias-handler", ConcreteHandler{})
	(*http.ServeMux).HandleFunc(mux, "/private-route-method-expression", HandlerFuncTarget)
	(*http.ServeMux).Handle(mux, "/private-route-method-expression-handler", ConcreteHandler{})

	fakeMux{}.HandleFunc("/private-route-fake", FakeUnregisteredHandler)
	fakeMux{}.Handle("/private-route-fake-handler", unknownHandler{})
	http.HandleFunc("/private-route-var", functionVariable)
	http.Handle("/private-route-interface", interfaceVariable)
	http.HandleFunc("/private-route-anonymous", func(http.ResponseWriter, *http.Request) {})
}
`,
	}

	for name, contents := range files {
		writeErr := root.WriteFile(name, []byte(contents), 0o600)
		if writeErr != nil {
			t.Fatal(writeErr)
		}
	}

	return root.Name()
}

func TestHTTPEntrypointsFollowBuildSelection(t *testing.T) {
	t.Parallel()
	root := httpEntrypointFixture(t)

	directory, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		closeErr := directory.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	}()

	for name, source := range map[string]string{
		"tagged.go": `//go:build otelplan_http_fixture

package httpentrypoints
import "net/http"
func Tagged(http.ResponseWriter, *http.Request) {}
func init() { http.HandleFunc("/tag-private-route", Tagged) }
`,
		"registration_test.go": `package httpentrypoints
import "net/http"
func testHandler(http.ResponseWriter, *http.Request) {}
func init() { http.HandleFunc("/test-private-route", testHandler) }
`,
	} {
		err := directory.WriteFile(name, []byte(source), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	checkHTTPBuildSelection(t, root, false)
	checkHTTPBuildSelection(t, root, true)
}

func checkHTTPBuildSelection(t *testing.T, root string, enabled bool) {
	t.Helper()

	var options discovery.Options

	options.Root, options.Offline, options.IncludeTests = root, true, enabled

	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}
	if enabled {
		options.BuildTags = []string{"otelplan_http_fixture"}
	}

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	for _, symbolID := range []model.SymbolID{
		"example.com/httpentrypoints.Tagged", "example.com/httpentrypoints.testHandler",
	} {
		symbol, exists := code.Symbol(symbolID)
		if exists != enabled {
			t.Fatalf("symbol %s presence=%t want=%t", symbolID, exists, enabled)
		}

		if enabled && !slices.Equal(symbol.Entrypoints, []string{"net/http.HandleFunc"}) {
			t.Fatalf("selected HTTP registration metadata missing for %s", symbolID)
		}
	}
}
