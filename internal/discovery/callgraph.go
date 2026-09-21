package discovery

import (
	"cmp"
	"context"
	"fmt"
	"go/types"
	"slices"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func addCallGraph(ctx context.Context, code *model.CodeModel, pkgs []*packages.Package) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("analyze call graph: %w", err)
	}

	program, _ := ssautil.Packages(pkgs, ssa.InstantiateGenerics)
	program.Build()

	err = ctx.Err()
	if err != nil {
		return fmt.Errorf("analyze call graph: %w", err)
	}

	graph := cha.CallGraph(program)
	callers := make(map[model.SymbolID]bool, len(code.Symbols))

	for _, symbol := range code.Symbols {
		callers[symbol.ID] = true
	}

	edges := map[model.CallRelation]bool{}

	for _, node := range graph.Nodes {
		err = ctx.Err()
		if err != nil {
			return fmt.Errorf("analyze call graph: %w", err)
		}

		collectNodeCalls(node, callers, edges)
	}

	code.CallEdges = make([]model.CallRelation, 0, len(edges))

	for edge := range edges {
		code.CallEdges = append(code.CallEdges, edge)
	}

	slices.SortFunc(code.CallEdges, func(left, right model.CallRelation) int {
		return cmp.Or(
			cmp.Compare(left.Caller, right.Caller),
			cmp.Compare(left.Callee, right.Callee),
			cmp.Compare(left.Precision, right.Precision),
		)
	})

	code.CallGraph = &model.CallGraphInfo{
		Algorithm: "cha", Scope: "analyzed-package-callers", Conservative: true,
		Limitations: []string{
			"Conservative candidates may be unreachable at runtime.",
			"Only analyzed package bodies are traversed; external callees are retained without traversing their bodies.",
			"Reflection and some generic dispatch may be missing.",
			"Calls in closures are attributed to the enclosing declaration.",
		},
	}

	return nil
}

func callSymbolID(function *ssa.Function) model.SymbolID {
	if function == nil {
		return ""
	}

	obj, isFunction := function.Object().(*types.Func)
	if !isFunction || obj == nil || obj.Pkg() == nil || obj.Name() == "init" {
		return ""
	}

	obj = obj.Origin()

	receiver := obj.Signature().Recv()
	if receiver == nil {
		return model.FunctionID(obj.Pkg().Path(), obj.Name())
	}

	typ := types.Unalias(receiver.Type())
	pointer := false

	if ptr, ok := typ.(*types.Pointer); ok {
		typ, pointer = types.Unalias(ptr.Elem()), true
	}

	named, ok := typ.(*types.Named)
	if !ok {
		return ""
	}

	canonicalReceiver := model.Receiver{Name: "", Type: named.Obj().Name(), Pointer: pointer}

	return model.MethodID(obj.Pkg().Path(), canonicalReceiver, obj.Name())
}

func collectNodeCalls(node *callgraph.Node, callers map[model.SymbolID]bool, edges map[model.CallRelation]bool) {
	owner := callOwner(node.Func)
	if owner == nil {
		return
	}

	caller := callSymbolID(owner)
	if !callers[caller] {
		return
	}

	for _, edge := range node.Out {
		callee := callSymbolID(edge.Callee.Func)
		if callee == "" {
			continue
		}

		precision := model.CallPrecisionConservative
		if owner == node.Func && edge.Site != nil && edge.Site.Common().StaticCallee() != nil {
			precision = model.CallPrecisionStatic
		}

		edges[model.CallRelation{Caller: caller, Callee: callee, Precision: precision}] = true
	}
}

func callOwner(function *ssa.Function) *ssa.Function {
	if function == nil || (function.Synthetic != "" && function.Origin() == nil && function.Parent() == nil) {
		return nil
	}

	for function.Object() == nil && function.Parent() != nil {
		function = function.Parent()
	}

	return function
}
