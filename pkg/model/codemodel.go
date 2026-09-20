package model

// ModuleInfo identifies a module in the loaded build and records its ownership
// and any replacement selected by the module graph.
type ModuleInfo struct {
	Path      string             `json:"path"`
	Version   string             `json:"version,omitempty"`
	Dir       string             `json:"dir,omitempty"`
	Main      bool               `json:"main"`
	Ownership Ownership          `json:"ownership"`
	Replace   *ModuleReplacement `json:"replace,omitempty"`
}

// ModuleReplacement describes the module path, version, or local directory
// that replaces a module in the effective build.
type ModuleReplacement struct {
	Path    string `json:"path"`
	Version string `json:"version,omitempty"`
	Dir     string `json:"dir,omitempty"`
}

// PackageInfo describes a discovered package and the source files that belong
// to it, including whether the package is generated or test-only.
type PackageInfo struct {
	ImportPath string   `json:"importPath"`
	Name       string   `json:"name"`
	Dir        string   `json:"dir,omitempty"`
	ModulePath string   `json:"modulePath,omitempty"`
	Files      []string `json:"files,omitempty"`
	Generated  bool     `json:"generated"`
	TestOnly   bool     `json:"testOnly"`
}

// InterfaceRelation records a concrete type that implements an interface and
// whether the relation is through the pointer method set.
type InterfaceRelation struct {
	InterfaceID  SymbolID `json:"interfaceID"`
	InterfacePkg string   `json:"interfacePkg"`
	Interface    string   `json:"interface"`
	ConcreteID   SymbolID `json:"concreteID"`
	Pointer      bool     `json:"pointer"`
}

// CallPrecision describes callee resolution, not proof that a call executes.
type CallPrecision string

// Call precision distinguishes a known callee from a conservative candidate.
const (
	CallPrecisionStatic       CallPrecision = "static"
	CallPrecisionConservative CallPrecision = "conservative"
)

// CallGraphInfo records the algorithm, scope, and limits of advisory call analysis.
type CallGraphInfo struct {
	Algorithm    string   `json:"algorithm"`
	Scope        string   `json:"scope"`
	Conservative bool     `json:"conservative"`
	Limitations  []string `json:"limitations"`
}

// CallRelation records a caller-to-callee edge and whether discovery resolved
// it statically or retained conservative candidates.
type CallRelation struct {
	Precision CallPrecision `json:"precision,omitempty"`
	Caller    SymbolID      `json:"caller"`
	Callee    SymbolID      `json:"callee"`
}

// InterfaceMethod maps an interface method to the concrete method selected for
// that implementation, distinguishing value and pointer concrete types.
type InterfaceMethod struct {
	InterfaceID SymbolID `json:"interfaceID"`
	ConcreteID  SymbolID `json:"concreteID"`
	Pointer     bool     `json:"pointer"`
	SymbolID    SymbolID `json:"symbol"`
}

// TypeField describes a field in a discovered type, including whether it is
// exported or embedded.
type TypeField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Exported bool   `json:"exported"`
	Embedded bool   `json:"embedded"`
}

// TypeImport records an import used while rendering a discovered type.
type TypeImport struct {
	Path  string `json:"path"`
	Alias string `json:"alias"`
}

// TypeInfo describes a discovered type expression, its kind, element type,
// imports, and fields.
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

// CodeModel contains normalized discovery metadata for an effective Go build.
// CallGraph and CallEdges are populated only when call analysis is requested.
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
	CallGraph        *CallGraphInfo      `json:"callGraph,omitempty"`
	CallEdges        []CallRelation      `json:"callEdges,omitempty"`
	BuildTags        []string            `json:"buildTags,omitempty"`
	GOOS             string              `json:"goos,omitempty"`
	GOARCH           string              `json:"goarch,omitempty"`
	EffectiveBuild   BuildEnvironment    `json:"effectiveBuild"`
}

// Symbol returns the exact symbol with id from the model, or false when the
// model has no such canonical identity. A found pointer refers to the model
// slice; modifying the symbol changes the model.
func (m *CodeModel) Symbol(id SymbolID) (*Symbol, bool) {
	for i := range m.Symbols {
		if m.Symbols[i].ID == id {
			return &m.Symbols[i], true
		}
	}

	return nil, false
}

// Implementors returns relations for the named interface package and type.
// The result preserves the relation entries stored in the model.
func (m *CodeModel) Implementors(interfacePath, interfaceName string) []InterfaceRelation {
	var out []InterfaceRelation

	for _, rel := range m.Implements {
		if rel.InterfacePkg == interfacePath && rel.Interface == interfaceName {
			out = append(out, rel)
		}
	}

	return out
}
