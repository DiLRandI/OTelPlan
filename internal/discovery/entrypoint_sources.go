package discovery

import (
	"context"
	"go/ast"
	"go/token"
	"go/types"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/tools/go/packages"
)

type entrypointExpressionSource struct {
	info       *types.Info
	expression ast.Expr
}

type entrypointFunctionSource struct {
	info      *types.Info
	signature *types.Signature
	body      *ast.BlockStmt
}

type entrypointSources struct {
	packages  map[string]*packages.Package
	indexed   map[string]bool
	values    map[types.Object]entrypointExpressionSource
	functions map[model.SymbolID]entrypointFunctionSource
	mutated   map[types.Object]bool
	addressed map[types.Object]bool
}

func newEntrypointSources(ctx context.Context, pkgs []*packages.Package) *entrypointSources {
	sources := &entrypointSources{packages: map[string]*packages.Package{}, indexed: map[string]bool{},
		values: map[types.Object]entrypointExpressionSource{}, functions: map[model.SymbolID]entrypointFunctionSource{},
		mutated: map[types.Object]bool{}, addressed: map[types.Object]bool{}}

	packages.Visit(pkgs, func(pkg *packages.Package) bool {
		previous := sources.packages[pkg.PkgPath]
		if previous == nil || len(pkg.Syntax) > len(previous.Syntax) {
			sources.packages[pkg.PkgPath] = pkg
		}

		return ctx.Err() == nil
	}, nil)

	if sources.packages[grpcImportPath] == nil && sources.packages[connectImportPath] == nil {
		return nil
	}

	for _, pkg := range pkgs {
		sources.indexPackage(ctx, pkg.PkgPath)
	}

	return sources
}

func (sources *entrypointSources) indexPackage(ctx context.Context, path string) {
	if sources.indexed[path] || ctx.Err() != nil {
		return
	}

	sources.indexed[path] = true

	pkg := sources.packages[path]
	if pkg == nil || pkg.TypesInfo == nil {
		return
	}

	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			if ctx.Err() != nil {
				return false
			}

			sources.indexNode(pkg.TypesInfo, node)

			return true
		})
	}
}

func (sources *entrypointSources) indexNode(info *types.Info, node ast.Node) {
	switch value := node.(type) {
	case *ast.FuncDecl:
		function, ok := info.Defs[value.Name].(*types.Func)
		if ok && value.Body != nil {
			sources.functions[functionSymbolID(function)] = entrypointFunctionSource{
				info: info, signature: function.Signature(), body: value.Body}
		}
	case *ast.ValueSpec:
		sources.indexValues(info, value)
	case *ast.AssignStmt:
		sources.indexAssignment(info, value)
	case *ast.RangeStmt:
		if value.Tok == token.ASSIGN {
			sources.mutated[entrypointBaseObject(info, value.Key)] = true
			sources.mutated[entrypointBaseObject(info, value.Value)] = true
		}
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			sources.addressed[entrypointBaseObject(info, value.X)] = true
		}
	}
}

func (sources *entrypointSources) indexValues(info *types.Info, value *ast.ValueSpec) {
	if len(value.Names) != len(value.Values) {
		return
	}

	for index, name := range value.Names {
		sources.values[info.Defs[name]] = entrypointExpressionSource{info: info, expression: value.Values[index]}
	}
}

func (sources *entrypointSources) indexAssignment(info *types.Info, assignment *ast.AssignStmt) {
	for index, left := range assignment.Lhs {
		name, identifier := left.(*ast.Ident)
		if identifier && assignment.Tok == token.DEFINE && info.Defs[name] != nil {
			if len(assignment.Lhs) == len(assignment.Rhs) {
				sources.values[info.Defs[name]] = entrypointExpressionSource{info: info, expression: assignment.Rhs[index]}
			}
		} else {
			sources.mutated[entrypointBaseObject(info, left)] = true
		}
	}
}

func entrypointBaseObject(info *types.Info, expression ast.Expr) types.Object {
	if expression == nil {
		return nil
	}

	switch value := ast.Unparen(expression).(type) {
	case *ast.Ident:
		return info.ObjectOf(value)
	case *ast.SelectorExpr:
		if object := info.ObjectOf(value.Sel); object != nil {
			if variable, ok := object.(*types.Var); ok && !variable.IsField() {
				return object
			}
		}

		return entrypointBaseObject(info, value.X)
	case *ast.IndexExpr:
		return entrypointBaseObject(info, value.X)
	case *ast.StarExpr:
		return entrypointBaseObject(info, value.X)
	default:
		return nil
	}
}

func (sources *entrypointSources) resolve(ctx context.Context,
	source entrypointExpressionSource,
) entrypointExpressionSource {
	seen := map[types.Object]bool{}

	for source.info != nil && source.expression != nil && ctx.Err() == nil {
		var advanced bool

		source, advanced = sources.resolveExpression(ctx, source, seen)
		if !advanced {
			return source
		}
	}

	return source
}

func (sources *entrypointSources) resolveExpression(ctx context.Context, source entrypointExpressionSource,
	seen map[types.Object]bool,
) (entrypointExpressionSource, bool) {
	source.expression = ast.Unparen(source.expression)

	switch value := source.expression.(type) {
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			source.expression = value.X

			return source, true
		}
	case *ast.CallExpr:
		if len(value.Args) == 1 && source.info.Types[value.Fun].IsType() {
			source.expression = value.Args[0]

			return source, true
		}
	case *ast.Ident, *ast.SelectorExpr:
		return sources.resolveInitializer(ctx, source, seen)
	}

	return source, false
}

func (sources *entrypointSources) resolveInitializer(ctx context.Context, source entrypointExpressionSource,
	seen map[types.Object]bool,
) (entrypointExpressionSource, bool) {
	object := entrypointBaseObject(source.info, source.expression)
	if object == nil || object.Pkg() == nil {
		return source, false
	}

	sources.indexPackage(ctx, object.Pkg().Path())

	if seen[object] || sources.mutated[object] {
		return entrypointExpressionSource{info: nil, expression: nil}, false
	}

	initializer, known := sources.values[object]
	seen[object] = true

	if !known {
		return source, false
	}

	return initializer, true
}

func (sources *entrypointSources) function(ctx context.Context, info *types.Info,
	expression ast.Expr,
) entrypointFunctionSource {
	expression = entrypointExpression(info, expression)
	if literal, ok := expression.(*ast.FuncLit); ok {
		signature, _ := info.TypeOf(literal).(*types.Signature)

		return entrypointFunctionSource{info: info, signature: signature, body: literal.Body}
	}

	function := entrypointFunction(info, expression)
	if function == nil || function.Pkg() == nil {
		return entrypointFunctionSource{info: nil, signature: nil, body: nil}
	}

	sources.indexPackage(ctx, function.Pkg().Path())

	return sources.functions[functionSymbolID(function)]
}
