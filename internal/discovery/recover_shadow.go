package discovery

import (
	"go/ast"

	"golang.org/x/tools/go/packages"
)

func recoverShadowed(pkg *packages.Package, file *ast.File) bool {
	if pkg.Types.Scope().Lookup("recover") != nil {
		return true
	}

	fileScope := pkg.TypesInfo.Scopes[file]

	return fileScope != nil && fileScope.Lookup("recover") != nil
}
