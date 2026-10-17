package discovery

import (
	"context"
	"fmt"
	"go/ast"
	"go/types"
	"slices"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

func addRecursionMetadata(ctx context.Context, code *model.CodeModel, pkgs []*packages.Package) error {
	indexes := make(map[model.SymbolID]int, len(code.Symbols))
	for index, symbol := range code.Symbols {
		indexes[symbol.ID] = index
	}

	edges := make([][]int, len(code.Symbols))
	for _, pkg := range pkgs {
		collectStaticCalls(ctx, pkg, indexes, edges)
	}

	order, err := recursiveCallOrder(ctx, edges)
	if err != nil {
		return fmt.Errorf("analyze recursive calls: %w", err)
	}

	return markRecursiveComponents(ctx, code, edges, order)
}

func markRecursiveComponents(ctx context.Context, code *model.CodeModel, edges [][]int, order []int) error {
	reverse := make([][]int, len(edges))
	for caller, callees := range edges {
		for _, callee := range callees {
			reverse[callee] = append(reverse[callee], caller)
		}
	}

	seen := make([]bool, len(edges))
	for _, start := range slices.Backward(order) {
		if seen[start] {
			continue
		}

		component, componentErr := recursiveCallComponent(ctx, reverse, seen, start)
		if componentErr != nil {
			return fmt.Errorf("analyze recursive calls: %w", componentErr)
		}

		if len(component) > 1 || slices.Contains(edges[start], start) {
			for _, member := range component {
				code.Symbols[member].Recursive = true
			}
		}
	}

	return nil
}

func collectStaticCalls(ctx context.Context, pkg *packages.Package, indexes map[model.SymbolID]int, edges [][]int) {
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

			caller, analyzed := indexes[functionSymbolID(object)]
			if !analyzed {
				continue
			}

			for _, callee := range staticFunctionCalls(ctx, pkg.TypesInfo, function.Body) {
				if index, found := indexes[functionSymbolID(callee)]; found {
					edges[caller] = append(edges[caller], index)
				}
			}
		}
	}
}

func staticFunctionCalls(ctx context.Context, info *types.Info, body *ast.BlockStmt) []*types.Func {
	var callees []*types.Func

	ast.Inspect(body, func(node ast.Node) bool {
		if ctx.Err() != nil {
			return false
		}

		if _, literal := node.(*ast.FuncLit); literal {
			return false
		}

		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		if callee := typeutil.StaticCallee(info, call); callee != nil {
			callees = append(callees, callee)
		}

		// A returned or stored closure does not execute inside its declaration.
		// Immediately invoked closures do and can complete a recursive cycle.
		if literal, immediate := ast.Unparen(call.Fun).(*ast.FuncLit); immediate {
			callees = append(callees, staticFunctionCalls(ctx, info, literal.Body)...)
		}

		return true
	})

	return callees
}

func recursiveCallOrder(ctx context.Context, edges [][]int) ([]int, error) {
	seen := make([]bool, len(edges))
	order := make([]int, 0, len(edges))

	var visit func(int) error

	visit = func(node int) error {
		if seen[node] {
			return nil
		}

		err := ctx.Err()
		if err != nil {
			return fmt.Errorf("order static calls: %w", err)
		}

		seen[node] = true
		for _, callee := range edges[node] {
			err = visit(callee)
			if err != nil {
				return err
			}
		}

		order = append(order, node)

		return nil
	}

	for node := range edges {
		err := visit(node)
		if err != nil {
			return nil, err
		}
	}

	err := ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("order static calls: %w", err)
	}

	return order, nil
}

func recursiveCallComponent(ctx context.Context, edges [][]int, seen []bool, start int) ([]int, error) {
	var members []int

	pending := []int{start}
	seen[start] = true

	for len(pending) > 0 {
		err := ctx.Err()
		if err != nil {
			return nil, fmt.Errorf("find recursive component: %w", err)
		}

		node := pending[len(pending)-1]
		pending = pending[:len(pending)-1]

		members = append(members, node)

		for _, callee := range edges[node] {
			if !seen[callee] {
				seen[callee] = true
				pending = append(pending, callee)
			}
		}
	}

	return members, nil
}
