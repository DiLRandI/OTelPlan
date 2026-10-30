package model

// SymbolID is the canonical identity of a function or method. Function IDs use
// import.path.Name; method IDs include the receiver form so pointer and value
// methods remain distinct.
type SymbolID string

// SymbolKind distinguishes free functions from methods.
type SymbolKind string

// Symbol kinds used in the code model.
const (
	SymbolFunction SymbolKind = "function"
	SymbolMethod   SymbolKind = "method"
)

// Visibility records whether a symbol is exported from its package.
type Visibility string

// Visibility values recorded for discovered symbols.
const (
	VisibilityExported   Visibility = "exported"
	VisibilityUnexported Visibility = "unexported"
)

// Ownership identifies whether a discovered symbol belongs to the application
// or a dependency.
type Ownership string

// Ownership values used by policy matching.
const (
	OwnershipApplication Ownership = "application"
	OwnershipDependency  Ownership = "dependency"
	OwnershipAny         Ownership = "any"
)

// SourceLocation identifies the source file and one-based line and column of a
// discovered declaration. Unknown positions use zero.
type SourceLocation struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Parameter describes one function or method parameter by source name and Go
// type expression.
type Parameter struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type"`
}

// Result describes one function or method result by source name and Go type
// expression.
type Result struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type"`
}

// Receiver describes the declared receiver type, optional source name, and
// whether the declaration uses a pointer receiver.
type Receiver struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Pointer bool   `json:"pointer"`
}

// GenericInfo records the type parameter names declared by a symbol.
type GenericInfo struct {
	TypeParams []string `json:"typeParams,omitempty"`
}

// Symbol is the normalized metadata used to select and explain one exact Go
// declaration, including signature, context and error result indexes.
type Symbol struct {
	ID                SymbolID       `json:"id"`
	Kind              SymbolKind     `json:"kind"`
	PackageImportPath string         `json:"package"`
	PackageName       string         `json:"packageName"`
	Name              string         `json:"name"`
	Receiver          *Receiver      `json:"receiver,omitempty"`
	Location          SourceLocation `json:"location"`
	Visibility        Visibility     `json:"visibility"`
	Parameters        []Parameter    `json:"parameters"`
	Results           []Result       `json:"results"`
	ContextIndexes    []int          `json:"contextIndexes,omitempty"`
	ErrorIndexes      []int          `json:"errorIndexes,omitempty"`
	Generics          *GenericInfo   `json:"generics,omitempty"`
	Generated         bool           `json:"generated"`
	TestFile          bool           `json:"testFile"`
	Ownership         Ownership      `json:"ownership"`
	Signature         string         `json:"signature"`
	HasBody           bool           `json:"hasBody"`
	Variadic          bool           `json:"variadic"`
	// Recursive identifies a cycle of statically resolved calls in analyzed bodies.
	Recursive bool `json:"recursive,omitempty"`
	// ManualSpan identifies a recognizable OpenTelemetry span-start call in the body.
	ManualSpan bool `json:"manualSpan,omitempty"`
	// RecoverShadowed identifies a package/file binding that hides Go's recover builtin.
	RecoverShadowed bool `json:"recoverShadowed,omitempty"`
	// Entrypoints records typed registration or handler-construction APIs referencing this declaration.
	Entrypoints []string `json:"entrypoints,omitempty"`
}

// ParsedSymbolID is the structured form of a canonical function or method ID.
type ParsedSymbolID struct {
	ImportPath string
	Receiver   *Receiver
	Name       string
}

// IsMethod reports whether the symbol represents a method declaration.
func (s Symbol) IsMethod() bool {
	return s.Kind == SymbolMethod
}

// HasContext reports whether discovery found one or more context parameter
// indexes on the symbol.
func (s Symbol) HasContext() bool {
	return len(s.ContextIndexes) > 0
}

// ReturnsError reports whether discovery found one or more error result
// indexes on the symbol.
func (s Symbol) ReturnsError() bool {
	return len(s.ErrorIndexes) > 0
}
