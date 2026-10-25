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

func TestDiscoveryIndexesTypedGRPCEntrypoints(t *testing.T) {
	t.Parallel()

	root := grpcEntrypointFixture(t)
	first := loadGRPCEntrypointModel(t, root)
	second := loadGRPCEntrypointModel(t, root)

	want := map[model.SymbolID][]string{
		"example.com/grpcentrypoints.(Implementation).Unary": {
			"google.golang.org/grpc.(*Server).RegisterService",
			"google.golang.org/grpc.(ServiceRegistrar).RegisterService",
		},
		"example.com/grpcentrypoints.(Implementation).Stream": {
			"google.golang.org/grpc.(*Server).RegisterService",
			"google.golang.org/grpc.(ServiceRegistrar).RegisterService",
		},
	}

	for symbolID, entrypoints := range want {
		assertGRPCEntrypoints(t, first, symbolID, entrypoints)
		assertGRPCEntrypoints(t, second, symbolID, entrypoints)
	}

	for _, symbolID := range []model.SymbolID{
		"example.com/grpcentrypoints.(Unregistered).Unary",
		"example.com/grpcentrypoints.(Unregistered).Stream",
		"example.com/grpcentrypoints.(Unregistered).ExtraRPC",
		"example.com/grpcentrypoints.(FakeRegistrar).RegisterService",
		"example.com/grpcentrypoints.(UnknownImplementation).UnknownUnary",
		"example.com/grpcentrypoints.(MutatedImplementation).Mutated",
	} {
		assertGRPCEntrypoints(t, first, symbolID, nil)
	}

	if !reflect.DeepEqual(first.Symbols, second.Symbols) {
		t.Fatal("gRPC entrypoint metadata was not deterministic")
	}

	encoded, err := json.Marshal(first.Symbols)
	if err != nil {
		t.Fatal(err)
	}

	for _, privateValue := range []string{
		"wire.UnrelatedName",
		"private metadata value",
		"private-wire-unary-value",
		"private-wire-stream-value",
	} {
		if strings.Contains(string(encoded), privateValue) {
			t.Fatalf("gRPC descriptor value leaked into symbol metadata: %s", privateValue)
		}
	}
}

func assertGRPCEntrypoints(t *testing.T, code *model.CodeModel, symbolID model.SymbolID, want []string) {
	t.Helper()

	symbol, found := code.Symbol(symbolID)
	if !found {
		t.Fatalf("missing gRPC fixture symbol %s", symbolID)
	}

	if !slices.Equal(symbol.Entrypoints, want) {
		t.Fatalf("gRPC entrypoints for %s = %v, want %v", symbolID, symbol.Entrypoints, want)
	}
}

func loadGRPCEntrypointModel(t *testing.T, root string) *model.CodeModel {
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

func grpcEntrypointFixture(t *testing.T) string {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := root.Close()
		if closeErr != nil {
			t.Errorf("close gRPC entrypoint fixture: %v", closeErr)
		}
	})

	files := map[string]string{
		"go.mod": `module example.com/grpcentrypoints

go 1.27

require google.golang.org/grpc v0.0.0

replace google.golang.org/grpc => ./grpc
`,
		"grpc/go.mod": `module google.golang.org/grpc

go 1.27
`,
		"grpc/grpc.go": `package grpc

import "context"

type ServiceRegistrar interface {
	RegisterService(*ServiceDesc, any)
}

type Server struct{}

func (*Server) RegisterService(*ServiceDesc, any) {}

type ServiceDesc struct {
	ServiceName string
	HandlerType any
	Methods     []MethodDesc
	Streams     []StreamDesc
	Metadata    any
}

type MethodDesc struct {
	MethodName string
	Handler    methodHandler
}

type StreamDesc struct {
	StreamName    string
	Handler       streamHandler
	ServerStreams bool
	ClientStreams bool
}

type UnaryServerInterceptor func(context.Context, any, UnaryHandler) (any, error)
type UnaryHandler func(context.Context, any) (any, error)
type methodHandler func(any, context.Context, func(any) error, UnaryServerInterceptor) (any, error)
type streamHandler func(any, ServerStream) error

type ServerStream interface {
	Context() context.Context
}
`,
		"api/generated.go": `package api

import (
	"context"

	grpc "google.golang.org/grpc"
)

type Service interface {
	Unary(context.Context) error
	Stream(grpc.ServerStream) error
}

func _WireUnary_Handler(
	srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor,
) (any, error) {
	err := srv.(Service).Unary(ctx)
	return nil, err
}

func _WireStream_Handler(srv any, stream grpc.ServerStream) error {
	return srv.(Service).Stream(stream)
}

var ServiceDesc = grpc.ServiceDesc{
	ServiceName: "wire.UnrelatedName",
	HandlerType: (*Service)(nil),
	Methods: []grpc.MethodDesc{{MethodName: "private-wire-unary-value", Handler: _WireUnary_Handler}},
	Streams: []grpc.StreamDesc{{
		StreamName: "private-wire-stream-value", Handler: _WireStream_Handler, ServerStreams: true,
	}},
	Metadata: "private metadata value",
}

func RegisterService(s grpc.ServiceRegistrar, srv Service) {
	s.RegisterService(&ServiceDesc, srv)
}

type UnknownService interface {
	UnknownUnary(context.Context) error
}

func _Unknown_Handler(
	srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor,
) (any, error) {
	return nil, srv.(UnknownService).UnknownUnary(ctx)
}

var UnknownServiceDesc = grpc.ServiceDesc{
	ServiceName: "wire.Unknown",
	HandlerType: (*UnknownService)(nil),
	Methods: []grpc.MethodDesc{{MethodName: "WireUnknown", Handler: _Unknown_Handler}},
}

func RegisterUnknownService(s grpc.ServiceRegistrar, srv UnknownService) {
	s.RegisterService(&UnknownServiceDesc, srv)
}

type MutatedService interface {
	Mutated(context.Context) error
}

func _Mutated_Handler(
	srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor,
) (any, error) {
	srv = nil
	return nil, srv.(MutatedService).Mutated(ctx)
}

var MutatedServiceDesc = grpc.ServiceDesc{
	ServiceName: "wire.Mutated",
	HandlerType: (*MutatedService)(nil),
	Methods: []grpc.MethodDesc{{MethodName: "WireMutated", Handler: _Mutated_Handler}},
}

func RegisterMutatedService(s grpc.ServiceRegistrar, srv MutatedService) {
	s.RegisterService(&MutatedServiceDesc, srv)
}
`,
		"app.go": `package grpcentrypoints

import (
	"context"

	api "example.com/grpcentrypoints/api"
	grpc "google.golang.org/grpc"
	aliasgrpc "google.golang.org/grpc"
)

type Implementation struct{}

func (Implementation) Unary(context.Context) error { return nil }
func (Implementation) Stream(grpc.ServerStream) error { return nil }

type Unregistered struct{}

func (Unregistered) Unary(context.Context) error { return nil }
func (Unregistered) Stream(grpc.ServerStream) error { return nil }
func (Unregistered) ExtraRPC(context.Context) error { return nil }

type UnknownImplementation struct{}

func (UnknownImplementation) UnknownUnary(context.Context) error { return nil }

type MutatedImplementation struct{}

func (MutatedImplementation) Mutated(context.Context) error { return nil }

type FakeRegistrar struct{}

func (FakeRegistrar) RegisterService(*grpc.ServiceDesc, any) {}

func registerUnknown(reg grpc.ServiceRegistrar, srv api.UnknownService) {
	api.RegisterUnknownService(reg, srv)
}

func registerAll() {
	var server grpc.Server
	server.RegisterService(&api.ServiceDesc, &Implementation{})

	var aliasServer aliasgrpc.Server
	aliasServer.RegisterService(&api.ServiceDesc, &Implementation{})

	api.RegisterService(&server, &Implementation{})
	var registrar grpc.ServiceRegistrar = &server
	api.RegisterService(registrar, &Implementation{})

	FakeRegistrar{}.RegisterService(&api.ServiceDesc, &Implementation{})
	var unknown api.UnknownService = &UnknownImplementation{}
	registerUnknown(&server, unknown)
	api.RegisterMutatedService(&server, &MutatedImplementation{})
}
`,
	}

	err = root.MkdirAll("api", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	err = root.MkdirAll("grpc", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	for name, contents := range files {
		writeErr := root.WriteFile(name, []byte(contents), 0o600)
		if writeErr != nil {
			t.Fatal(writeErr)
		}
	}

	return root.Name()
}
