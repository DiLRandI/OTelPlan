package discovery

import (
	"context"
	"fmt"
	"go/ast"
	"go/types"
	"slices"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/tools/go/packages"
)

type entrypointCandidate struct {
	function    *types.Func
	integration string
}

type entrypointRecognizer func(*types.Info, *ast.CallExpr) []entrypointCandidate

func addEntrypointMetadata(ctx context.Context, code *model.CodeModel, pkgs []*packages.Package) error {
	symbols := make(map[model.SymbolID]*model.Symbol, len(code.Symbols))
	for index := range code.Symbols {
		symbol := &code.Symbols[index]
		symbols[symbol.ID] = symbol
	}

	recognizers := []entrypointRecognizer{httpEntrypoints}

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			collectEntrypoints(ctx, pkg.TypesInfo, file, recognizers, symbols)
		}
	}

	for _, symbol := range symbols {
		slices.Sort(symbol.Entrypoints)
		symbol.Entrypoints = slices.Compact(symbol.Entrypoints)
	}

	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("analyze technical entrypoints: %w", err)
	}

	return nil
}

func collectEntrypoints(ctx context.Context, info *types.Info, file *ast.File,
	recognizers []entrypointRecognizer, symbols map[model.SymbolID]*model.Symbol,
) {
	ast.Inspect(file, func(node ast.Node) bool {
		if ctx.Err() != nil {
			return false
		}

		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		for _, recognize := range recognizers {
			for _, candidate := range recognize(info, call) {
				symbol := symbols[functionSymbolID(candidate.function)]
				if symbol != nil {
					symbol.Entrypoints = append(symbol.Entrypoints, candidate.integration)
				}
			}
		}

		return true
	})
}
