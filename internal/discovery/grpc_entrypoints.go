package discovery

import (
	"context"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"
)

const grpcImportPath = "google.golang.org/grpc"

type grpcRegistration struct {
	descriptor  entrypointExpressionSource
	service     entrypointExpressionSource
	integration string
}

func (sources *entrypointSources) grpcEntrypoints(ctx context.Context, info *types.Info,
	call *ast.CallExpr,
) []entrypointCandidate {
	registrar, _ := typeutil.Callee(info, call).(*types.Func)

	var registrations []grpcRegistration
	if grpcRegistrar(registrar) && len(call.Args) >= 2 {
		registrations = []grpcRegistration{{
			descriptor:  entrypointExpressionSource{info: info, expression: call.Args[len(call.Args)-2]},
			service:     entrypointExpressionSource{info: info, expression: call.Args[len(call.Args)-1]},
			integration: string(functionSymbolID(registrar)),
		}}
	} else {
		registrations = sources.grpcWrapperRegistrations(ctx, info, call)
	}

	candidates := make([]entrypointCandidate, 0, len(registrations))
	for _, registration := range registrations {
		candidates = append(candidates, sources.grpcCandidates(ctx, registration)...)
	}

	return candidates
}

func grpcRegistrar(function *types.Func) bool {
	if function == nil || function.Pkg() == nil || function.Pkg().Path() != grpcImportPath ||
		function.Name() != "RegisterService" || function.Signature().Recv() == nil {
		return false
	}

	typ := types.Unalias(function.Signature().Recv().Type())
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = types.Unalias(pointer.Elem())
	}

	return grpcNamedType(typ, "Server") || grpcNamedType(typ, "ServiceRegistrar")
}

func (sources *entrypointSources) grpcWrapperRegistrations(ctx context.Context, info *types.Info,
	call *ast.CallExpr,
) []grpcRegistration {
	callee := typeutil.StaticCallee(info, call)
	if callee == nil {
		return nil
	}

	definition := sources.function(ctx, info, call.Fun)
	if definition.body == nil || definition.signature == nil || definition.signature.Variadic() {
		return nil
	}

	var registrations []grpcRegistration

	ast.Inspect(definition.body, func(node ast.Node) bool {
		if ctx.Err() != nil {
			return false
		}

		registration, found := sources.grpcForwardedRegistration(definition, node, info, call)
		if found {
			registrations = append(registrations, registration)
		}

		return true
	})

	return registrations
}

func (sources *entrypointSources) grpcForwardedRegistration(definition entrypointFunctionSource,
	node ast.Node, info *types.Info, call *ast.CallExpr,
) (grpcRegistration, bool) {
	var unknown grpcRegistration

	registration, found := node.(*ast.CallExpr)
	if !found || len(registration.Args) < 2 {
		return unknown, false
	}

	registrar, _ := typeutil.Callee(definition.info, registration).(*types.Func)
	if !grpcRegistrar(registrar) {
		return unknown, false
	}

	variable, found := ast.Unparen(registration.Args[len(registration.Args)-1]).(*ast.Ident)
	if !found {
		return unknown, false
	}

	object := definition.info.ObjectOf(variable)
	argument := forwardedEntrypointArgument(definition.signature, object, info, call)

	if argument == nil || sources.mutated[object] || sources.addressed[object] {
		return unknown, false
	}

	return grpcRegistration{
		descriptor: entrypointExpressionSource{
			info: definition.info, expression: registration.Args[len(registration.Args)-2],
		},
		service:     entrypointExpressionSource{info: info, expression: argument},
		integration: string(functionSymbolID(registrar)),
	}, true
}

func forwardedEntrypointArgument(signature *types.Signature, object types.Object,
	info *types.Info, call *ast.CallExpr,
) ast.Expr {
	offset := 0

	if selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok {
		selection := info.Selections[selector]
		if selection != nil && selection.Kind() == types.MethodExpr {
			offset = 1
		}
	}

	for index := range signature.Params().Len() {
		if signature.Params().At(index) == object && index+offset < len(call.Args) {
			return call.Args[index+offset]
		}
	}

	return nil
}

func (sources *entrypointSources) grpcCandidates(ctx context.Context,
	registration grpcRegistration,
) []entrypointCandidate {
	expression := concreteEntrypointExpression(registration.service.info, registration.service.expression)

	implementation := registration.service.info.TypeOf(expression)
	if implementation == nil {
		return nil
	}

	if _, dynamic := implementation.Underlying().(*types.Interface); dynamic {
		return nil
	}

	descriptor := sources.resolve(ctx, registration.descriptor)

	literal, found := descriptor.expression.(*ast.CompositeLit)
	if !found || !grpcNamedType(descriptor.info.TypeOf(literal), "ServiceDesc") {
		return nil
	}

	fields := entrypointLiteralFields(descriptor.info, literal)

	contract := grpcServiceContract(fields["HandlerType"])
	if contract == nil || !types.Implements(implementation, contract) {
		return nil
	}

	var candidates []entrypointCandidate

	for _, field := range []string{"Methods", "Streams"} {
		for _, handler := range sources.grpcDescriptorHandlers(ctx, fields[field]) {
			for _, function := range sources.grpcHandlerMethods(ctx, handler, implementation) {
				candidates = append(candidates, entrypointCandidate{function: function, integration: registration.integration})
			}
		}
	}

	return candidates
}

func grpcServiceContract(handlerType entrypointExpressionSource) *types.Interface {
	if handlerType.expression == nil {
		return nil
	}

	pointer, ok := types.Unalias(handlerType.info.TypeOf(handlerType.expression)).(*types.Pointer)
	if !ok {
		return nil
	}

	contract, _ := types.Unalias(pointer.Elem()).Underlying().(*types.Interface)

	return contract
}

func grpcNamedType(typ types.Type, name string) bool {
	if typ == nil {
		return false
	}

	named, ok := types.Unalias(typ).(*types.Named)

	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == grpcImportPath && named.Obj().Name() == name
}

func entrypointLiteralFields(info *types.Info, literal *ast.CompositeLit) map[string]entrypointExpressionSource {
	fields := map[string]entrypointExpressionSource{}

	for _, element := range literal.Elts {
		pair, found := element.(*ast.KeyValueExpr)
		if !found {
			continue
		}

		key, ok := pair.Key.(*ast.Ident)
		if ok {
			fields[key.Name] = entrypointExpressionSource{info: info, expression: pair.Value}
		}
	}

	return fields
}

func (sources *entrypointSources) grpcDescriptorHandlers(ctx context.Context, source entrypointExpressionSource,
) []entrypointFunctionSource {
	source = sources.resolve(ctx, source)

	literal, ok := source.expression.(*ast.CompositeLit)
	if !ok {
		return nil
	}

	var handlers []entrypointFunctionSource

	for _, expression := range literal.Elts {
		if pair, keyed := expression.(*ast.KeyValueExpr); keyed {
			expression = pair.Value
		}

		entry := sources.resolve(ctx, entrypointExpressionSource{info: source.info, expression: expression})

		method, ok := entry.expression.(*ast.CompositeLit)
		if !ok {
			continue
		}

		typ := entry.info.TypeOf(method)
		if !grpcNamedType(typ, "MethodDesc") && !grpcNamedType(typ, "StreamDesc") {
			continue
		}

		handler := entrypointLiteralFields(entry.info, method)["Handler"]
		if handler.expression != nil {
			handlers = append(handlers, sources.function(ctx, handler.info, handler.expression))
		}
	}

	return handlers
}

func (sources *entrypointSources) grpcHandlerMethods(ctx context.Context, handler entrypointFunctionSource,
	implementation types.Type,
) []*types.Func {
	if handler.body == nil || handler.signature == nil || handler.signature.Params().Len() == 0 {
		return nil
	}

	service := handler.signature.Params().At(0)
	if sources.mutated[service] || sources.addressed[service] {
		return nil
	}

	var methods []*types.Func

	ast.Inspect(handler.body, func(node ast.Node) bool {
		if ctx.Err() != nil {
			return false
		}

		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		method := grpcBoundMethod(handler.info, call, service, implementation)
		if method != nil {
			methods = append(methods, method)
		}

		return true
	})

	return methods
}

func grpcBoundMethod(info *types.Info, call *ast.CallExpr, service types.Object,
	implementation types.Type,
) *types.Func {
	selector, found := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !found {
		return nil
	}

	assertion, assertedReceiver := ast.Unparen(selector.X).(*ast.TypeAssertExpr)
	if !assertedReceiver || assertion.Type == nil {
		return nil
	}

	variable, ok := ast.Unparen(assertion.X).(*ast.Ident)
	if !ok || info.ObjectOf(variable) != service {
		return nil
	}

	asserted := info.TypeOf(assertion.Type)
	if asserted == nil || !types.AssignableTo(implementation, asserted) {
		return nil
	}

	called, _ := typeutil.Callee(info, call).(*types.Func)
	if called == nil {
		return nil
	}

	object, _, _ := types.LookupFieldOrMethod(implementation, true, nil, called.Name())
	method, _ := object.(*types.Func)

	return method
}
