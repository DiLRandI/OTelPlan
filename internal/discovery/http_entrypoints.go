package discovery

import (
	"context"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"
)

const httpImportPath = "net/http"

func httpEntrypoints(_ context.Context, info *types.Info, call *ast.CallExpr) []entrypointCandidate {
	registrar := typeutil.StaticCallee(info, call)
	if !httpRegistrar(registrar) || len(call.Args) == 0 {
		return nil
	}

	argument := call.Args[len(call.Args)-1]

	var handler *types.Func
	if registrar.Name() == "HandleFunc" {
		handler = entrypointFunction(info, entrypointExpression(info, argument))
	} else {
		handler = httpHandlerFunction(info, argument)
	}

	if handler == nil {
		return nil
	}

	return []entrypointCandidate{{function: handler, integration: string(functionSymbolID(registrar))}}
}

func httpRegistrar(function *types.Func) bool {
	if function == nil || function.Pkg() == nil || function.Pkg().Path() != httpImportPath {
		return false
	}

	if function.Name() != "Handle" && function.Name() != "HandleFunc" {
		return false
	}

	receiver := function.Signature().Recv()
	if receiver == nil {
		return true
	}

	return httpNamedType(receiver.Type(), "ServeMux")
}

func entrypointExpression(info *types.Info, expression ast.Expr) ast.Expr {
	for {
		expression = ast.Unparen(expression)

		call, conversion := expression.(*ast.CallExpr)
		if !conversion || len(call.Args) != 1 || !info.Types[call.Fun].IsType() {
			return expression
		}

		expression = call.Args[0]
	}
}

func entrypointFunction(info *types.Info, expression ast.Expr) *types.Func {
	switch value := expression.(type) {
	case *ast.Ident:
		function, _ := info.ObjectOf(value).(*types.Func)

		return function
	case *ast.SelectorExpr:
		function, _ := info.ObjectOf(value.Sel).(*types.Func)

		return function
	case *ast.IndexExpr:
		return entrypointFunction(info, ast.Unparen(value.X))
	case *ast.IndexListExpr:
		return entrypointFunction(info, ast.Unparen(value.X))
	default:
		return nil
	}
}

func httpHandlerMethod(typ types.Type) *types.Func {
	if typ == nil {
		return nil
	}

	object, _, _ := types.LookupFieldOrMethod(typ, true, nil, "ServeHTTP")
	function, _ := object.(*types.Func)

	return function
}

func httpHandlerFunction(info *types.Info, expression ast.Expr) *types.Func {
	expression = concreteEntrypointExpression(info, expression)
	if httpNamedType(info.TypeOf(expression), "HandlerFunc") {
		return entrypointFunction(info, entrypointExpression(info, expression))
	}

	return httpHandlerMethod(info.TypeOf(expression))
}

func concreteEntrypointExpression(info *types.Info, expression ast.Expr) ast.Expr {
	for {
		expression = ast.Unparen(expression)

		call, conversion := expression.(*ast.CallExpr)
		if !conversion || len(call.Args) != 1 || !info.Types[call.Fun].IsType() {
			return expression
		}

		typ := info.TypeOf(call)
		if typ == nil {
			return expression
		}

		if _, isInterface := typ.Underlying().(*types.Interface); !isInterface {
			return expression
		}

		expression = call.Args[0]
	}
}

func httpNamedType(typ types.Type, name string) bool {
	if typ == nil {
		return false
	}

	typ = types.Unalias(typ)
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = types.Unalias(pointer.Elem())
	}

	named, ok := typ.(*types.Named)

	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == httpImportPath && named.Obj().Name() == name
}
