package model

import (
	"errors"
	"fmt"
	"strings"
)

// SymbolID is the canonical identity of a function or method. Function IDs use
// import.path.Name; method IDs include the receiver form so pointer and value
// methods remain distinct.
type SymbolID string

const (
	receiverOpen  = "("
	receiverClose = ")"
	pointerPrefix = "*"
	separator     = "."
)

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
}

// FunctionID constructs the canonical ID for a package-level function.
func FunctionID(importPath, name string) SymbolID {
	return SymbolID(importPath + separator + name)
}

// MethodID constructs the canonical ID for a method, preserving whether its
// receiver is a pointer.
func MethodID(importPath string, recv Receiver, method string) SymbolID {
	recvType := recv.Type
	if recv.Pointer {
		recvType = pointerPrefix + recvType
	}

	return SymbolID(importPath + separator + receiverOpen + recvType + receiverClose + separator + method)
}

// ParsedSymbolID is the structured form of a canonical function or method ID.
type ParsedSymbolID struct {
	ImportPath string
	Receiver   *Receiver
	Name       string
}

// ParseSymbolID validates and splits a canonical symbol ID into its import
// path, optional receiver, and declaration name.
func ParseSymbolID(id SymbolID) (ParsedSymbolID, error) {
	s := string(id)
	if s == "" {
		return ParsedSymbolID{}, errors.New("empty symbol id")
	}

	if idx := strings.LastIndex(s, receiverOpen); idx > 0 && s[idx-1] == separator[0] {
		closeRel := strings.Index(s[idx:], receiverClose)
		if closeRel < 0 {
			return ParsedSymbolID{}, fmt.Errorf("malformed method symbol id %q: unterminated receiver", s)
		}

		idxClose := idx + closeRel
		recvPart := s[idx+1 : idxClose]

		rest := s[idxClose+1:]
		if !strings.HasPrefix(rest, separator) || len(rest) == 1 {
			return ParsedSymbolID{}, fmt.Errorf("malformed method symbol id %q", s)
		}

		method := rest[1:]
		recv := &Receiver{Type: strings.TrimPrefix(recvPart, pointerPrefix)}

		recv.Pointer = strings.HasPrefix(recvPart, pointerPrefix)

		if recv.Type == "" {
			return ParsedSymbolID{}, fmt.Errorf("malformed method symbol id %q: empty receiver type", s)
		}

		return ParsedSymbolID{ImportPath: s[:idx-1], Receiver: recv, Name: method}, nil
	}

	idx := strings.LastIndex(s, separator)
	if idx <= 0 || idx == len(s)-1 {
		return ParsedSymbolID{}, fmt.Errorf("malformed function symbol id %q", s)
	}

	return ParsedSymbolID{ImportPath: s[:idx], Name: s[idx+1:]}, nil
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
