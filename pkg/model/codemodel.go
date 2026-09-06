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

type TypeInfo struct {
	Type    string      `json:"type"`
	Kind    string      `json:"kind"`
	Element string      `json:"element,omitempty"`
	Fields  []TypeField `json:"fields,omitempty"`
}

type CodeModel struct {
	GoVersion        string              `json:"goVersion"`
	ModuleRoot       string              `json:"moduleRoot"`
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
