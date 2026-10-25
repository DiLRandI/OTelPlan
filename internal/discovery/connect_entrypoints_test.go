package discovery_test

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/internal/testenv"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestDiscoveryIndexesTypedConnectEntrypoints(t *testing.T) {
	t.Parallel()

	root := connectEntrypointFixture(t)
	first := loadConnectEntrypointModel(t, root)
	second := loadConnectEntrypointModel(t, root)

	want := map[model.SymbolID][]string{
		"example.com/connectentrypoints.(Implementation).Unary":       {"connectrpc.com/connect.NewUnaryHandler"},
		"example.com/connectentrypoints.(Implementation).UnarySimple": {"connectrpc.com/connect.NewUnaryHandlerSimple"},
		"example.com/connectentrypoints.(Implementation).Client":      {"connectrpc.com/connect.NewClientStreamHandler"},
		"example.com/connectentrypoints.(Implementation).ClientSimple": {
			"connectrpc.com/connect.NewClientStreamHandlerSimple",
		},
		"example.com/connectentrypoints.(Implementation).Server": {"connectrpc.com/connect.NewServerStreamHandler"},
		"example.com/connectentrypoints.(Implementation).ServerSimple": {
			"connectrpc.com/connect.NewServerStreamHandlerSimple",
		},
		"example.com/connectentrypoints.(Implementation).Bidi":    {"connectrpc.com/connect.NewBidiStreamHandler"},
		"example.com/connectentrypoints.(Implementation).Wrapped": {"connectrpc.com/connect.NewUnaryHandler"},
		"example.com/connectentrypoints.(Embedded).Promoted":      {"connectrpc.com/connect.NewUnaryHandler"},
		"example.com/connectentrypoints.(Concrete).Bound":         {"connectrpc.com/connect.NewUnaryHandler"},
		"example.com/connectentrypoints.(Concrete).private":       {"connectrpc.com/connect.NewUnaryHandler"},
	}
	for symbolID, entrypoints := range want {
		assertConnectEntrypoints(t, first, symbolID, entrypoints)
		assertConnectEntrypoints(t, second, symbolID, entrypoints)
	}

	for _, symbolID := range []model.SymbolID{
		"example.com/connectentrypoints.Unregistered",
		"example.com/connectentrypoints.Stored",
		"example.com/connectentrypoints.Anonymous",
		"example.com/connectentrypoints.(Implementation).Changed",
		"example.com/connectentrypoints.(Implementation).Addressed",
		"example.com/connectentrypoints.(Implementation).Unknown",
		"example.com/connectentrypoints.(Implementation).Field",
	} {
		assertConnectEntrypoints(t, first, symbolID, nil)
	}

	if !reflect.DeepEqual(first.Symbols, second.Symbols) {
		t.Fatal("Connect entrypoint metadata was not deterministic")
	}

	encoded, err := json.Marshal(first.Symbols)
	if err != nil {
		t.Fatal(err)
	}

	for _, privateValue := range []string{"private/procedure", "private/simple", "private/options", "private/wrapped"} {
		if strings.Contains(string(encoded), privateValue) {
			t.Fatalf("Connect procedure or option value leaked into symbol metadata: %s", privateValue)
		}
	}
}

func assertConnectEntrypoints(t *testing.T, code *model.CodeModel, symbolID model.SymbolID, want []string) {
	t.Helper()

	symbol, found := code.Symbol(symbolID)
	if !found {
		t.Fatalf("missing Connect fixture symbol %s", symbolID)
	}

	if !slices.Equal(symbol.Entrypoints, want) {
		t.Fatalf("Connect entrypoints for %s = %v, want %v", symbolID, symbol.Entrypoints, want)
	}
}

func loadConnectEntrypointModel(t *testing.T, root string) *model.CodeModel {
	t.Helper()

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	return code
}

func TestConnectEntrypointCacheReadOnly(t *testing.T) {
	t.Parallel()

	var options discovery.Options

	options.Root, options.Offline, options.CacheDir = connectEntrypointFixture(t), true, t.TempDir()
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	warm, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	before := testenv.SnapshotFiles(t, options.CacheDir)
	options.CacheReadOnly = true

	cached, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(connectInventoryJSON(t, warm), connectInventoryJSON(t, cached)) {
		t.Fatal("cached Connect metadata differs from fresh analysis")
	}

	testenv.AssertFilesUnchanged(t, before, testenv.SnapshotFiles(t, options.CacheDir))
}

func TestConnectEntrypointRelocation(t *testing.T) {
	t.Parallel()

	root := connectEntrypointFixture(t)
	before := connectInventoryJSON(t, loadConnectEntrypointModel(t, root))
	relocated := t.TempDir()

	err := os.CopyFS(relocated, os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(before, connectInventoryJSON(t, loadConnectEntrypointModel(t, relocated))) {
		t.Fatal("checkout relocation changed Connect inventory")
	}
}

func connectInventoryJSON(t *testing.T, code *model.CodeModel) []byte {
	t.Helper()

	encoded, err := json.Marshal(code.Symbols)
	if err != nil {
		t.Fatal(err)
	}

	return encoded
}

func connectEntrypointFixture(t *testing.T) string {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := root.Close()
		if closeErr != nil {
			t.Errorf("close Connect entrypoint fixture: %v", closeErr)
		}
	})

	err = root.MkdirAll("connect", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	err = root.MkdirAll("api", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	writeConnectFixtureFile(t, root, "go.mod", `module example.com/connectentrypoints

go 1.27

require connectrpc.com/connect v0.0.0

replace connectrpc.com/connect => ./connect
`)
	writeConnectFixtureFile(t, root, "connect/go.mod", `module connectrpc.com/connect

go 1.27
`)
	writeConnectFixtureFile(t, root, "connect/handler.go", `package connect

import "context"

type Handler struct{}
type HandlerOption interface{}
type Request[T any] struct{}
type Response[T any] struct{}
type ClientStream[T any] struct{}
type ServerStream[T any] struct{}
type BidiStream[Req, Res any] struct{}

func NewUnaryHandler[Req, Res any](
	string, func(context.Context, *Request[Req]) (*Response[Res], error), ...HandlerOption,
) *Handler {
	return nil
}
func NewUnaryHandlerSimple[Req, Res any](string, func(context.Context, *Req) (*Res, error), ...HandlerOption) *Handler {
	return nil
}
func NewClientStreamHandler[Req, Res any](
	string, func(context.Context, *ClientStream[Req]) (*Response[Res], error), ...HandlerOption,
) *Handler {
	return nil
}
func NewClientStreamHandlerSimple[Req, Res any](
	string, func(context.Context, *ClientStream[Req]) (*Res, error), ...HandlerOption,
) *Handler {
	return nil
}
func NewServerStreamHandler[Req, Res any](
	string, func(context.Context, *Request[Req], *ServerStream[Res]) error, ...HandlerOption,
) *Handler {
	return nil
}
func NewServerStreamHandlerSimple[Req, Res any](
	string, func(context.Context, *Req, *ServerStream[Res]) error, ...HandlerOption,
) *Handler {
	return nil
}
func NewBidiStreamHandler[Req, Res any](
	string, func(context.Context, *BidiStream[Req, Res]) error, ...HandlerOption,
) *Handler {
	return nil
}
`)
	writeConnectFixtureFile(t, root, "api/generated.go", `package api

import (
	"context"
	connect "connectrpc.com/connect"
)

type Request struct{}
type Response struct{}
type Service interface {
	Wrapped(context.Context, *connect.Request[Request]) (*connect.Response[Response], error)
}

type ChangedService interface {
	Changed(context.Context, *connect.Request[Request]) (*connect.Response[Response], error)
}
type AddressedService interface {
	Addressed(context.Context, *connect.Request[Request]) (*connect.Response[Response], error)
}
type UnknownService interface {
	Unknown(context.Context, *connect.Request[Request]) (*connect.Response[Response], error)
}

func NewHandler(service Service, options ...connect.HandlerOption) *connect.Handler {
	return connect.NewUnaryHandler[Request, Response]("private/wrapped", service.Wrapped, options...)
}
func NewChangedHandler(service ChangedService) *connect.Handler {
	service = nil
	return connect.NewUnaryHandler[Request, Response]("private/changed", service.Changed)
}
func NewAddressedHandler(service AddressedService) *connect.Handler {
	_ = &service
	return connect.NewUnaryHandler[Request, Response]("private/addressed", service.Addressed)
}
func NewUnknownHandler(service UnknownService) *connect.Handler {
	return connect.NewUnaryHandler[Request, Response]("private/unknown", service.Unknown)
}
`)
	writeConnectFixtureFile(t, root, "app.go", `package connectentrypoints

import (
	"context"
	connect "connectrpc.com/connect"
	connectalias "connectrpc.com/connect"
	api "example.com/connectentrypoints/api"
)

type Req struct{}
type Res struct{}
type Implementation struct{}

func (Implementation) Unary(context.Context, *connect.Request[Req]) (*connect.Response[Res], error) { return nil, nil }
func (Implementation) UnarySimple(context.Context, *Req) (*Res, error) { return nil, nil }
func (Implementation) Client(
	context.Context, *connect.ClientStream[Req],
) (*connect.Response[Res], error) { return nil, nil }
func (Implementation) ClientSimple(context.Context, *connect.ClientStream[Req]) (*Res, error) { return nil, nil }
func (Implementation) Server(context.Context, *connect.Request[Req], *connect.ServerStream[Res]) error { return nil }
func (Implementation) ServerSimple(context.Context, *Req, *connect.ServerStream[Res]) error { return nil }
func (Implementation) Bidi(context.Context, *connect.BidiStream[Req, Res]) error { return nil }
func (Implementation) Wrapped(
	context.Context, *connect.Request[api.Request],
) (*connect.Response[api.Response], error) { return nil, nil }
func (Implementation) Changed(
context.Context, *connect.Request[api.Request],
) (*connect.Response[api.Response], error) { return nil, nil }
func (Implementation) Addressed(
context.Context, *connect.Request[api.Request],
) (*connect.Response[api.Response], error) { return nil, nil }
func (Implementation) Unknown(
context.Context, *connect.Request[api.Request],
) (*connect.Response[api.Response], error) { return nil, nil }
func (Implementation) Field(context.Context, *connect.Request[Req]) (*connect.Response[Res], error) { return nil, nil }

type Embedded struct{}
func (Embedded) Promoted(context.Context, *connect.Request[Req]) (*connect.Response[Res], error) { return nil, nil }
type Concrete struct{ Embedded }
func (Concrete) Bound(context.Context, *connect.Request[Req]) (*connect.Response[Res], error) { return nil, nil }
func (Concrete) private(context.Context, *connect.Request[Req]) (*connect.Response[Res], error) { return nil, nil }

func Unregistered(context.Context, *connect.Request[Req]) (*connect.Response[Res], error) { return nil, nil }
func Stored(context.Context, *connect.Request[Req]) (*connect.Response[Res], error) { return nil, nil }
var stored = Stored
func Anonymous(context.Context, *connect.Request[Req]) (*connect.Response[Res], error) { return nil, nil }

type fakeConnect struct{}
func (fakeConnect) NewUnaryHandler(
	string, func(context.Context, *connect.Request[Req]) (*connect.Response[Res], error), ...connect.HandlerOption,
) *connect.Handler { return nil }
type callbackFields struct {
	Handler func(context.Context, *connect.Request[Req]) (*connect.Response[Res], error)
}

type privateService interface {
	private(context.Context, *connect.Request[Req]) (*connect.Response[Res], error)
}

func newPrivateHandler(service privateService) *connect.Handler {
	return connect.NewUnaryHandler[Req, Res]("private/private", service.private)
}

func register() {
	implementation := Implementation{}
	connect.NewUnaryHandler[Req, Res]("private/procedure", implementation.Unary, connect.HandlerOption(nil))
	connectalias.NewUnaryHandlerSimple[Req, Res](
		"private/simple", (func(context.Context, *Req) (*Res, error))(implementation.UnarySimple),
		connectalias.HandlerOption(nil),
	)
	connect.NewClientStreamHandler[Req, Res]("private/client", implementation.Client, connect.HandlerOption(nil))
	connect.NewClientStreamHandlerSimple[Req, Res](
		"private/client-simple", implementation.ClientSimple, connect.HandlerOption(nil),
	)
	connect.NewServerStreamHandler[Req, Res]("private/server", implementation.Server, connect.HandlerOption(nil))
	connect.NewServerStreamHandlerSimple[Req, Res](
		"private/server-simple", implementation.ServerSimple, connect.HandlerOption(nil),
	)
	connect.NewBidiStreamHandler[Req, Res]("private/bidi", implementation.Bidi, connect.HandlerOption(nil))

	concrete := Concrete{}
	connect.NewUnaryHandler[Req, Res]("private/bound", concrete.Bound)
	connect.NewUnaryHandler[Req, Res]("private/promoted", concrete.Promoted)
	connect.NewUnaryHandler[Req, Res]("private/stored", stored)
	connect.NewUnaryHandler[Req, Res]("private/anonymous",
		func(ctx context.Context, request *connect.Request[Req]) (*connect.Response[Res], error) {
			return Anonymous(ctx, request)
		})
	fields := callbackFields{Handler: implementation.Field}
	connect.NewUnaryHandler[Req, Res]("private/field", fields.Handler)
	fakeConnect{}.NewUnaryHandler("private/fake", Unregistered)

	api.NewHandler(&implementation, connect.HandlerOption(nil))
	api.NewChangedHandler(&implementation)
	api.NewAddressedHandler(&implementation)
	registerUnknown(&implementation)
	registerPrivate(&concrete)
}

func registerUnknown(service api.UnknownService) { api.NewUnknownHandler(service) }
func registerPrivate(service *Concrete) { newPrivateHandler(service) }
`)

	return root.Name()
}

func writeConnectFixtureFile(t *testing.T, root *os.Root, name, contents string) {
	t.Helper()

	err := root.WriteFile(name, []byte(contents), 0o600)
	if err != nil {
		t.Fatal(err)
	}
}
