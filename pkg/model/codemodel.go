package model

type ModuleInfo struct {
	Path      string             `json:"path"`
	Version   string             `json:"version,omitempty"`
	Dir       string             `json:"dir,omitempty"`
	Main      bool               `json:"main"`
	Ownership Ownership          `json:"ownership"`
	Replace   *ModuleReplacement `json:"replace,omitempty"`
}

type ModuleReplacement struct {
	Path    string `json:"path"`
	Version string `json:"version,omitempty"`
	Dir     string `json:"dir,omitempty"`
}

type PackageInfo struct {
	ImportPath string   `json:"importPath"`
	Name       string   `json:"name"`
	Dir        string   `json:"dir,omitempty"`
	ModulePath string   `json:"modulePath,omitempty"`
	Files      []string `json:"files,omitempty"`
	Generated  bool     `json:"generated"`
	TestOnly   bool     `json:"testOnly"`
}

type InterfaceRelation struct {
	InterfaceID  SymbolID `json:"interfaceID"`
	InterfacePkg string   `json:"interfacePkg"`
	Interface    string   `json:"interface"`
	ConcreteID   SymbolID `json:"concreteID"`
	Pointer      bool     `json:"pointer"`
}

type CallRelation struct {
	Caller SymbolID `json:"caller"`
	Callee SymbolID `json:"callee"`
}

type InterfaceMethod struct {
	InterfaceID SymbolID `json:"interfaceID"`
	ConcreteID  SymbolID `json:"concreteID"`
	Pointer     bool     `json:"pointer"`
	SymbolID    SymbolID `json:"symbol"`
}

type TypeField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Exported bool   `json:"exported"`
	Embedded bool   `json:"embedded"`
}

type TypeImport struct {
	Path  string `json:"path"`
	Alias string `json:"alias"`
}

type TypeInfo struct {
	Expression string       `json:"expression,omitempty"`
	Imports    []TypeImport `json:"imports,omitempty"`
	Type       string       `json:"type"`
	Kind       string       `json:"kind"`
	Element    string       `json:"element,omitempty"`
	Fields     []TypeField  `json:"fields,omitempty"`
}

// BuildEnvironment describes the Go inputs that can change package selection
// or type checking. Lock fingerprints use normalized manifest contents rather
// than the paths retained here for reading the effective manifests.
type BuildEnvironment struct {
	GoVersion     string   `json:"goVersion"`
	GOOS          string   `json:"goos"`
	GOARCH        string   `json:"goarch"`
	BuildTags     []string `json:"buildTags,omitempty"`
	ModuleMode    string   `json:"moduleMode"`
	ModFile       string   `json:"modFile,omitempty"`
	Workspace     bool     `json:"workspace"`
	CGOEnabled    string   `json:"cgoEnabled,omitempty"`
	GOEXPERIMENT  string   `json:"goexperiment,omitempty"`
	GOFIPS140     string   `json:"gofips140,omitempty"`
	GOAMD64       string   `json:"goamd64,omitempty"`
	GOARM         string   `json:"goarm,omitempty"`
	GOARM64       string   `json:"goarm64,omitempty"`
	GO386         string   `json:"go386,omitempty"`
	GOMIPS        string   `json:"gomips,omitempty"`
	GOMIPS64      string   `json:"gomips64,omitempty"`
	GOPPC64       string   `json:"goppc64,omitempty"`
	GORISCV64     string   `json:"goriscv64,omitempty"`
	GOWASM        string   `json:"gowasm,omitempty"`
	CGOCFLAGS     string   `json:"cgoCFlags,omitempty"`
	CGOCPPFLAGS   string   `json:"cgoCPPFlags,omitempty"`
	CGOLDFLAGS    string   `json:"cgoLDFlags,omitempty"`
	CGOFFLAGS     string   `json:"cgoFFlags,omitempty"`
	CC            string   `json:"cc,omitempty"`
	CXX           string   `json:"cxx,omitempty"`
	CGOCXXFLAGS   string   `json:"cgoCXXFlags,omitempty"`
	SemanticFlags []string `json:"semanticFlags,omitempty"`
}

type CodeModel struct {
	GoVersion        string              `json:"goVersion"`
	ModuleRoot       string              `json:"moduleRoot"`
	WorkspaceFile    string              `json:"workspaceFile,omitempty"`
	Modules          []ModuleInfo        `json:"modules"`
	Packages         []PackageInfo       `json:"packages"`
	Symbols          []Symbol            `json:"symbols"`
	Types            []TypeInfo          `json:"types,omitempty"`
	Implements       []InterfaceRelation `json:"implements,omitempty"`
	InterfaceMethods []InterfaceMethod   `json:"interfaceMethods,omitempty"`
	CallEdges        []CallRelation      `json:"callEdges,omitempty"`
	BuildTags        []string            `json:"buildTags,omitempty"`
	GOOS             string              `json:"goos,omitempty"`
	GOARCH           string              `json:"goarch,omitempty"`
	EffectiveBuild   BuildEnvironment    `json:"effectiveBuild"`
}

func (m *CodeModel) Symbol(id SymbolID) (*Symbol, bool) {
	for i := range m.Symbols {
		if m.Symbols[i].ID == id {
			return &m.Symbols[i], true
		}
	}
	return nil, false
}

func (m *CodeModel) Implementors(interfacePath, interfaceName string) []InterfaceRelation {
	var out []InterfaceRelation
	for _, rel := range m.Implements {
		if rel.InterfacePkg == interfacePath && rel.Interface == interfaceName {
			out = append(out, rel)
		}
	}
	return out
}
