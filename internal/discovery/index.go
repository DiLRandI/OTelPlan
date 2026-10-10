package discovery

import (
	"errors"
	"go/ast"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/tools/go/packages"
)

var errReceiverUnknown = errors.New("cannot determine receiver type")

type builder struct {
	types     map[string]model.TypeInfo
	modules   map[string]*model.ModuleInfo
	packages  []*packages.Package
	fileFlags map[string]fileFlags
}

type fileFlags struct {
	generated bool
	test      bool
}

func buildModel(pkgs, all []*packages.Package, opts Options) *model.CodeModel {
	indexer := &builder{
		types:     map[string]model.TypeInfo{},
		modules:   map[string]*model.ModuleInfo{},
		packages:  pkgs,
		fileFlags: map[string]fileFlags{},
	}

	code := new(model.CodeModel)
	code.GoVersion = opts.goVersion
	code.ModuleRoot = opts.Root
	code.WorkspaceFile = opts.workspaceFile
	code.GOOS, code.GOARCH = opts.GOOS, opts.GOARCH
	code.EffectiveBuild = opts.effectiveBuild

	if len(opts.BuildTags) > 0 {
		code.BuildTags = append([]string(nil), opts.BuildTags...)
	}

	for _, pkg := range all {
		indexer.collectModule(code, pkg)
	}

	for _, pkg := range pkgs {
		indexer.collectPackage(code, pkg)
	}

	for _, pkg := range pkgs {
		indexer.collectSymbols(code, pkg)
	}

	sort.Slice(code.Modules, func(i, j int) bool { return code.Modules[i].Path < code.Modules[j].Path })
	sort.Strings(code.BuildTags)
	indexer.collectInterfaceRelations(code)

	for _, info := range indexer.types {
		code.Types = append(code.Types, info)
	}

	sort.Slice(code.Types, func(i, j int) bool { return code.Types[i].Type < code.Types[j].Type })
	sort.Slice(code.Symbols, func(i, j int) bool { return code.Symbols[i].ID < code.Symbols[j].ID })

	return code
}

func (indexer *builder) collectModule(code *model.CodeModel, pkg *packages.Package) {
	if pkg.Module == nil {
		return
	}

	if _, exists := indexer.modules[pkg.Module.Path]; exists {
		return
	}

	info := new(model.ModuleInfo)
	info.Path, info.Version, info.Dir = pkg.Module.Path, pkg.Module.Version, pkg.Module.Dir
	info.Main = pkg.Module.Main
	info.Ownership = model.OwnershipApplication

	if !pkg.Module.Main {
		info.Ownership = model.OwnershipDependency
	}

	if replacement := pkg.Module.Replace; replacement != nil {
		info.Replace = &model.ModuleReplacement{Path: replacement.Path, Version: replacement.Version, Dir: replacement.Dir}
	}

	indexer.modules[pkg.Module.Path] = info
	code.Modules = append(code.Modules, *info)
}

func (indexer *builder) collectPackage(code *model.CodeModel, pkg *packages.Package) {
	if len(pkg.GoFiles) == 0 && len(pkg.Syntax) == 0 {
		return
	}

	var info model.PackageInfo

	info.ImportPath, info.Name = pkg.PkgPath, pkg.Name

	if pkg.Module != nil {
		info.ModulePath = pkg.Module.Path
	}

	for _, f := range pkg.Syntax {
		pos := pkg.Fset.Position(f.Pos())
		flags := fileFlags{generated: ast.IsGenerated(f), test: strings.HasSuffix(pos.Filename, "_test.go")}
		indexer.fileFlags[pos.Filename] = flags

		info.Files = append(info.Files, relFile(pkg, pos.Filename))
	}

	sort.Strings(info.Files)
	code.Packages = append(code.Packages, info)
}

func (indexer *builder) collectSymbols(code *model.CodeModel, pkg *packages.Package) {
	for _, f := range pkg.Syntax {
		for _, decl := range f.Decls {
			declaration, exists := decl.(*ast.FuncDecl)
			if !exists {
				continue
			}

			sym := indexer.symbolFromDecl(pkg, declaration)
			if sym == nil {
				continue
			}

			code.Symbols = append(code.Symbols, *sym)
		}
	}
}

func (indexer *builder) symbolFromDecl(pkg *packages.Package, declaration *ast.FuncDecl) *model.Symbol {
	if declaration.Name.Name == "_" || declaration.Name.Name == "init" {
		return nil
	}

	obj, exists := pkg.TypesInfo.Defs[declaration.Name]
	if !exists || obj == nil {
		return nil
	}

	pos := pkg.Fset.Position(declaration.Pos())
	flags := indexer.fileFlags[pos.Filename]

	sym := new(model.Symbol)
	sym.PackageImportPath, sym.PackageName = pkg.PkgPath, pkg.Name
	sym.Name = declaration.Name.Name
	sym.HasBody = declaration.Body != nil
	sym.Location = model.SourceLocation{File: relFile(pkg, pos.Filename), Line: pos.Line, Column: pos.Column}
	sym.Visibility = model.VisibilityExported
	sym.Ownership = ownership(pkg)
	sym.Generated, sym.TestFile = flags.generated, flags.test

	if !declaration.Name.IsExported() {
		sym.Visibility = model.VisibilityUnexported
	}

	sig, exists := obj.Type().(*types.Signature)
	if !exists {
		return nil
	}

	sym.Signature = sig.String()

	sym.Variadic = sig.Variadic()

	collectDeclarationTypeParameters(sym, sig)

	if declaration.Recv != nil {
		recv, err := indexer.receiver(pkg, declaration)
		if err != nil {
			return nil
		}

		sym.Kind = model.SymbolMethod
		sym.Receiver = recv
		sym.ID = model.MethodID(pkg.PkgPath, *recv, declaration.Name.Name)
	} else {
		sym.Kind = model.SymbolFunction
		sym.ID = model.FunctionID(pkg.PkgPath, declaration.Name.Name)
	}

	indexer.collectDeclarationParameters(sym, sig)
	indexer.collectDeclarationResults(sym, sig)

	return sym
}

func collectDeclarationTypeParameters(sym *model.Symbol, sig *types.Signature) {
	for _, params := range []*types.TypeParamList{sig.TypeParams(), sig.RecvTypeParams()} {
		for tparam := range params.TypeParams() {
			if sym.Generics == nil {
				sym.Generics = new(model.GenericInfo)
			}

			sym.Generics.TypeParams = append(sym.Generics.TypeParams, types.TypeString(tparam, nil))
		}
	}
}

func (indexer *builder) collectDeclarationParameters(sym *model.Symbol, sig *types.Signature) {
	params := sig.Params()
	for parameterIndex := range params.Len() {
		parameter := params.At(parameterIndex)

		sym.Parameters = append(sym.Parameters, model.Parameter{
			Name: parameter.Name(),
			Type: indexer.collectType(parameter.Type()),
		})

		if isContextType(parameter.Type()) {
			sym.ContextIndexes = append(sym.ContextIndexes, parameterIndex)
		}
	}
}

func (indexer *builder) collectDeclarationResults(sym *model.Symbol, sig *types.Signature) {
	errorType := types.Universe.Lookup("error").Type()

	results := sig.Results()
	for resultIndex := range results.Len() {
		result := results.At(resultIndex)

		sym.Results = append(sym.Results, model.Result{
			Name: result.Name(),
			Type: indexer.collectType(result.Type()),
		})

		if types.Identical(types.Unalias(result.Type()), errorType) {
			sym.ErrorIndexes = append(sym.ErrorIndexes, resultIndex)
		}
	}
}

func (indexer *builder) receiver(pkg *packages.Package, declaration *ast.FuncDecl) (*model.Receiver, error) {
	obj, exists := pkg.TypesInfo.Defs[declaration.Name]
	if !exists || obj == nil {
		return nil, errReceiverUnknown
	}

	sig, exists := obj.Type().(*types.Signature)
	if !exists || sig.Recv() == nil {
		return nil, errReceiverUnknown
	}

	recvName := ""

	recvField := sig.Recv()

	if named, exists := recvField.Type().(*types.Named); exists {
		recvName = named.Obj().Name()
	} else if pointer, exists := recvField.Type().(*types.Pointer); exists {
		if named, exists := pointer.Elem().(*types.Named); exists {
			recvName = named.Obj().Name()
		}
	}

	if recvName == "" {
		return nil, errReceiverUnknown
	}

	return &model.Receiver{
		Name:    recvField.Name(),
		Type:    recvName,
		Pointer: isPointerReceiver(recvField.Type()),
	}, nil
}

func isPointerReceiver(t types.Type) bool {
	_, exists := t.(*types.Pointer)

	return exists
}

func isContextType(t types.Type) bool {
	named, exists := types.Unalias(t).(*types.Named)
	if !exists {
		return false
	}

	obj := named.Obj()

	return obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == "context" && obj.Name() == "Context"
}

func relFile(pkg *packages.Package, file string) string {
	if pkg.Module != nil && pkg.Module.Dir != "" {
		rel, err := filepath.Rel(pkg.Module.Dir, file)
		if err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}

	return filepath.ToSlash(file)
}

func ownership(pkg *packages.Package) model.Ownership {
	if pkg.Module != nil && pkg.Module.Main {
		return model.OwnershipApplication
	}

	return model.OwnershipDependency
}
