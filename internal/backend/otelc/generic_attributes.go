package otelc

import (
	"go/ast"
	"go/parser"

	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const (
	accessorArgumentSource = "argument"
	accessorResultSource   = "result"
)

func genericCaptureIssue(code *model.CodeModel, symbol *model.Symbol, attributes []model.AttributePlan) string {
	if len(attributes) == 0 {
		return ""
	}

	types := make(map[string]model.TypeInfo, len(code.Types))
	for _, typ := range code.Types {
		types[typ.Type] = typ
	}

	typeParameters := make(map[string]bool, len(symbol.Generics.TypeParams))
	for _, name := range symbol.Generics.TypeParams {
		typeParameters[name] = true
	}

	for _, attribute := range attributes {
		access, err := validate.AttributeAccessor(code, *symbol, attribute.From)
		if err != nil {
			return "generic attribute source cannot be resolved safely"
		}

		if access.Source == "constant" {
			continue
		}

		var rootType string
		if access.Source == accessorArgumentSource {
			rootType = symbol.Parameters[access.Index].Type
		} else {
			rootType = symbol.Results[access.Index].Type
		}

		if issue := genericTypeExpressionIssue(types[rootType].Expression, typeParameters); issue != "" {
			return issue
		}
	}

	return ""
}

func genericTypeExpressionIssue(expression string, names map[string]bool) string {
	if expression == "" {
		return "generic attribute source is missing its Go type expression"
	}

	parsed, err := parser.ParseExpr(expression)
	if err != nil {
		return "generic attribute source has an invalid Go type expression"
	}

	if expressionUsesTypeParameters(parsed, names) {
		return "generic attribute source requires an accessor with unbound type parameters"
	}

	return ""
}

func expressionUsesTypeParameters(expression ast.Expr, names map[string]bool) bool {
	found := false

	var visit func(ast.Node) bool

	visit = func(node ast.Node) bool {
		if found {
			return false
		}

		switch value := node.(type) {
		case *ast.SelectorExpr:
			// Qualified type names and field identifiers do not bind the target's type parameters.
			ast.Inspect(value.X, visit)

			return false
		case *ast.Field:
			ast.Inspect(value.Type, visit)

			return false
		case *ast.Ident:
			found = names[value.Name]
		}

		return !found
	}
	ast.Inspect(expression, visit)

	return found
}
