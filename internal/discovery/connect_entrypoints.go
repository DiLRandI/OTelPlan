package discovery

import (
	"context"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"
)

const connectImportPath = "connectrpc.com/connect"

func connectConstructor(function *types.Func) bool {
	if function == nil || function.Pkg() == nil || function.Pkg().Path() != connectImportPath ||
		function.Signature().Recv() != nil {
		return false
	}

	switch function.Name() {
	case "NewUnaryHandler", "NewUnaryHandlerSimple", "NewClientStreamHandler", "NewClientStreamHandlerSimple",
		"NewServerStreamHandler", "NewServerStreamHandlerSimple", "NewBidiStreamHandler":
		return true
	default:
		return false
	}
}

func (sources *entrypointSources) connectEntrypoints(ctx context.Context, info *types.Info,
	call *ast.CallExpr,
) []entrypointCandidate {
	constructor := typeutil.StaticCallee(info, call)
	if connectConstructor(constructor) && len(call.Args) >= 2 {
		handler := entrypointFunction(info, entrypointExpression(info, call.Args[1]))
		if handler == nil {
			return nil
		}

		return []entrypointCandidate{{function: handler, integration: string(functionSymbolID(constructor))}}
	}

	definition := sources.function(ctx, info, call.Fun)
	if definition.body == nil || definition.signature == nil {
		return nil
	}

	return sources.connectFactoryCandidates(ctx, definition, info, call)
}

func (sources *entrypointSources) connectFactoryCandidates(ctx context.Context,
	definition entrypointFunctionSource, info *types.Info, call *ast.CallExpr,
) []entrypointCandidate {
	var candidates []entrypointCandidate

	ast.Inspect(definition.body, func(node ast.Node) bool {
		if ctx.Err() != nil {
			return false
		}

		construction, found := node.(*ast.CallExpr)
		if !found || len(construction.Args) < 2 {
			return true
		}

		factory := typeutil.StaticCallee(definition.info, construction)
		if !connectConstructor(factory) {
			return true
		}

		handler := sources.connectForwardedMethod(definition, construction.Args[1], info, call)
		if handler != nil {
			candidates = append(candidates, entrypointCandidate{
				function: handler, integration: string(functionSymbolID(factory)),
			})
		}

		return true
	})

	return candidates
}

func (sources *entrypointSources) connectForwardedMethod(definition entrypointFunctionSource,
	callback ast.Expr, info *types.Info, call *ast.CallExpr,
) *types.Func {
	selector, found := entrypointExpression(definition.info, callback).(*ast.SelectorExpr)
	if !found {
		return nil
	}

	receiver, found := ast.Unparen(selector.X).(*ast.Ident)
	if !found {
		return nil
	}

	object := definition.info.ObjectOf(receiver)
	argument := forwardedEntrypointArgument(definition.signature, object, info, call)

	if argument == nil || sources.mutated[object] || sources.addressed[object] {
		return nil
	}

	implementation := info.TypeOf(concreteEntrypointExpression(info, argument))
	if implementation == nil {
		return nil
	}

	if _, dynamic := implementation.Underlying().(*types.Interface); dynamic {
		return nil
	}

	declared := entrypointFunction(definition.info, selector)
	if declared == nil || !types.AssignableTo(implementation, object.Type()) {
		return nil
	}

	method, _, _ := types.LookupFieldOrMethod(implementation, true, declared.Pkg(), declared.Name())
	function, _ := method.(*types.Func)

	return function
}
