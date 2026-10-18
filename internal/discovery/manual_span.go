package discovery

import (
	"context"
	"fmt"
	"go/ast"
	"go/types"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

const otelTraceImportPath = "go.opentelemetry.io/otel/trace"

func addManualSpanMetadata(ctx context.Context, code *model.CodeModel, pkgs []*packages.Package) error {
	symbols := make(map[model.SymbolID]*model.Symbol, len(code.Symbols))
	for index := range code.Symbols {
		symbol := &code.Symbols[index]
		symbols[symbol.ID] = symbol
	}

	for _, pkg := range pkgs {
		tracer := findTracerInterface(ctx, pkg.Types, make(map[*types.Package]bool))

		err := ctx.Err()
		if err != nil {
			return fmt.Errorf("analyze manual spans: %w", err)
		}

		if tracer != nil {
			collectManualSpans(ctx, pkg, tracer, symbols)
		}
	}

	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("analyze manual spans: %w", err)
	}

	return nil
}

func findTracerInterface(ctx context.Context, pkg *types.Package,
	seen map[*types.Package]bool) *types.Interface {
	if ctx.Err() != nil || pkg == nil || seen[pkg] {
		return nil
	}

	seen[pkg] = true
	if pkg.Path() == otelTraceImportPath {
		return tracerInterfaceType(pkg)
	}

	for _, imported := range pkg.Imports() {
		tracer := findTracerInterface(ctx, imported, seen)
		if tracer != nil {
			return tracer
		}
	}

	return nil
}

func tracerInterfaceType(pkg *types.Package) *types.Interface {
	object, isType := pkg.Scope().Lookup("Tracer").(*types.TypeName)
	if !isType {
		return nil
	}

	tracer, _ := types.Unalias(object.Type()).Underlying().(*types.Interface)

	return tracer
}

func collectManualSpans(ctx context.Context, pkg *packages.Package, tracer *types.Interface,
	symbols map[model.SymbolID]*model.Symbol) {
	for _, file := range pkg.Syntax {
		for _, declaration := range file.Decls {
			if ctx.Err() != nil {
				return
			}

			function, isFunction := declaration.(*ast.FuncDecl)
			if !isFunction || function.Body == nil {
				continue
			}

			object, hasFunction := pkg.TypesInfo.Defs[function.Name].(*types.Func)
			if !hasFunction {
				continue
			}

			symbol := symbols[functionSymbolID(object)]
			if symbol != nil {
				symbol.ManualSpan = containsManualSpan(ctx, pkg.TypesInfo, function.Body, tracer)
			}
		}
	}
}

func containsManualSpan(ctx context.Context, info *types.Info, body *ast.BlockStmt, tracer *types.Interface) bool {
	found := false

	inspectFunctionCalls(ctx, body, func(call *ast.CallExpr) {
		callee, isFunction := typeutil.Callee(info, call).(*types.Func)
		if !isFunction || callee.Name() != "Start" {
			return
		}

		receiver := callee.Signature().Recv()
		if receiver != nil && types.Implements(receiver.Type(), tracer) {
			found = true
		}
	})

	return found
}

func inspectFunctionCalls(ctx context.Context, body *ast.BlockStmt, visit func(*ast.CallExpr)) {
	ast.Inspect(body, func(node ast.Node) bool {
		if ctx.Err() != nil {
			return false
		}

		if _, literal := node.(*ast.FuncLit); literal {
			return false
		}

		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}

		visit(call)

		// Stored or returned closure bodies belong to a later invocation.
		// Immediately invoked closures can contribute calls to this declaration.
		if literal, immediate := ast.Unparen(call.Fun).(*ast.FuncLit); immediate {
			inspectFunctionCalls(ctx, literal.Body, visit)
		}

		return true
	})
}
