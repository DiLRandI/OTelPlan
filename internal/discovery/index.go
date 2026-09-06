package discovery

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/tools/go/packages"
)

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
	b := &builder{
		types:     map[string]model.TypeInfo{},
		modules:   map[string]*model.ModuleInfo{},
		packages:  pkgs,
		fileFlags: map[string]fileFlags{},
	}
	m := &model.CodeModel{
		GoVersion:     opts.goVersion,
		ModuleRoot:    opts.Root,
		WorkspaceFile: opts.workspaceFile,
		GOOS:          opts.GOOS,
		GOARCH:        opts.GOARCH,
	}
	if len(opts.BuildTags) > 0 {
		m.BuildTags = append([]string(nil), opts.BuildTags...)
	}
	for _, p := range all {
		b.collectModule(m, p)
	}
	for _, p := range pkgs {
		b.collectPackage(m, p)
	}
	for _, p := range pkgs {
		b.collectSymbols(m, p)
	}
	sort.Slice(m.Modules, func(i, j int) bool { return m.Modules[i].Path < m.Modules[j].Path })
	sort.Strings(m.BuildTags)
	b.collectInterfaceRelations(m)
	for _, info := range b.types {
		m.Types = append(m.Types, info)
	}
	sort.Slice(m.Types, func(i, j int) bool { return m.Types[i].Type < m.Types[j].Type })
	sort.Slice(m.Symbols, func(i, j int) bool { return m.Symbols[i].ID < m.Symbols[j].ID })
	return m
}

func (b *builder) collectModule(m *model.CodeModel, p *packages.Package) {
	if p.Module == nil {
		return
	}
	if _, ok := b.modules[p.Module.Path]; ok {
		return
	}
	info := &model.ModuleInfo{
		Path:      p.Module.Path,
		Version:   p.Module.Version,
		Dir:       p.Module.Dir,
		Main:      p.Module.Main,
		Ownership: model.OwnershipApplication,
	}
	if !p.Module.Main {
		info.Ownership = model.OwnershipDependency
	}
	if replacement := p.Module.Replace; replacement != nil {
		info.Replace = &model.ModuleReplacement{Path: replacement.Path, Version: replacement.Version, Dir: replacement.Dir}
	}
	b.modules[p.Module.Path] = info
	m.Modules = append(m.Modules, *info)
}

func (b *builder) collectPackage(m *model.CodeModel, p *packages.Package) {
	if len(p.GoFiles) == 0 && len(p.Syntax) == 0 {
		return
	}
	info := model.PackageInfo{
		ImportPath: p.PkgPath,
		Name:       p.Name,
	}
	if p.Module != nil {
		info.ModulePath = p.Module.Path
	}
	for _, f := range p.Syntax {
		pos := p.Fset.Position(f.Pos())
		flags := fileFlags{generated: ast.IsGenerated(f), test: strings.HasSuffix(pos.Filename, "_test.go")}
		b.fileFlags[pos.Filename] = flags
		info.Files = append(info.Files, relFile(p, pos.Filename))
	}
	sort.Strings(info.Files)
	m.Packages = append(m.Packages, info)
}

func (b *builder) collectSymbols(m *model.CodeModel, p *packages.Package) {
	for _, f := range p.Syntax {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			sym := b.symbolFromDecl(p, fn)
			if sym == nil {
				continue
			}
			m.Symbols = append(m.Symbols, *sym)
		}
	}
}

func (b *builder) symbolFromDecl(p *packages.Package, fn *ast.FuncDecl) *model.Symbol {
	if fn.Name.Name == "_" || fn.Name.Name == "init" {
		return nil
	}
	obj, ok := p.TypesInfo.Defs[fn.Name]
	if !ok || obj == nil {
		return nil
	}
	pos := p.Fset.Position(fn.Pos())
	flags := b.fileFlags[pos.Filename]

	sym := &model.Symbol{
		PackageImportPath: p.PkgPath,
		PackageName:       p.Name,
		Name:              fn.Name.Name,
		Location: model.SourceLocation{
			File:   relFile(p, pos.Filename),
			Line:   pos.Line,
			Column: pos.Column,
		},
		Visibility: model.VisibilityExported,
		Ownership:  ownership(p),
		Generated:  flags.generated,
		TestFile:   flags.test,
	}
	if !fn.Name.IsExported() {
		sym.Visibility = model.VisibilityUnexported
	}

	sig, ok := obj.Type().(*types.Signature)
	if !ok {
		return nil
	}
	sym.Signature = sig.String()
	for _, params := range []*types.TypeParamList{sig.TypeParams(), sig.RecvTypeParams()} {
		for i := 0; i < params.Len(); i++ {
			if sym.Generics == nil {
				sym.Generics = &model.GenericInfo{}
			}
			sym.Generics.TypeParams = append(sym.Generics.TypeParams, types.TypeString(params.At(i), nil))
		}
	}

	if fn.Recv != nil {
		recv, err := b.receiver(p, fn)
		if err != nil {
			return nil
		}
		sym.Kind = model.SymbolMethod
		sym.Receiver = recv
		sym.ID = model.MethodID(p.PkgPath, *recv, fn.Name.Name)
	} else {
		sym.Kind = model.SymbolFunction
		sym.ID = model.FunctionID(p.PkgPath, fn.Name.Name)
	}

	params := sig.Params()
	for i := 0; i < params.Len(); i++ {
		pv := params.At(i)
		sym.Parameters = append(sym.Parameters, model.Parameter{
			Name: pv.Name(),
			Type: b.collectType(pv.Type()),
		})
		if isContextType(pv.Type()) {
			sym.ContextIndexes = append(sym.ContextIndexes, i)
		}
	}
	results := sig.Results()
	for i := 0; i < results.Len(); i++ {
		rv := results.At(i)
		sym.Results = append(sym.Results, model.Result{
			Name: rv.Name(),
			Type: b.collectType(rv.Type()),
		})
		if types.Identical(types.Unalias(rv.Type()), errorType) {
			sym.ErrorIndexes = append(sym.ErrorIndexes, i)
		}
	}

	return sym
}

func (b *builder) receiver(p *packages.Package, fn *ast.FuncDecl) (*model.Receiver, error) {
	obj, ok := p.TypesInfo.Defs[fn.Name]
	if !ok || obj == nil {
		return nil, errReceiverUnknown
	}
	sig, ok := obj.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return nil, errReceiverUnknown
	}
	recvName := ""
	recvField := sig.Recv()
	if named, ok := recvField.Type().(*types.Named); ok {
		recvName = named.Obj().Name()
	} else if pointer, ok := recvField.Type().(*types.Pointer); ok {
		if named, ok := pointer.Elem().(*types.Named); ok {
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

var errReceiverUnknown = errUnknownReceiver{}

type errUnknownReceiver struct{}

func (errUnknownReceiver) Error() string { return "cannot determine receiver type" }

func isPointerReceiver(t types.Type) bool {
	_, ok := t.(*types.Pointer)
	return ok
}

func isContextType(t types.Type) bool {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == "context" && obj.Name() == "Context"
}

var errorType = types.Universe.Lookup("error").Type()

func relFile(p *packages.Package, file string) string {
	if p.Module != nil && p.Module.Dir != "" {
		if rel, err := filepath.Rel(p.Module.Dir, file); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(file)
}

func ownership(p *packages.Package) model.Ownership {
	if p.Module != nil && p.Module.Main {
		return model.OwnershipApplication
	}
	return model.OwnershipDependency
}
