package model

import (
	"fmt"
	"strings"
)

type SymbolID string

const (
	receiverOpen  = "("
	receiverClose = ")"
	pointerPrefix = "*"
	separator     = "."
)

type SymbolKind string

const (
	SymbolFunction SymbolKind = "function"
	SymbolMethod   SymbolKind = "method"
)

type Visibility string

const (
	VisibilityExported   Visibility = "exported"
	VisibilityUnexported Visibility = "unexported"
)

type Ownership string

const (
	OwnershipApplication Ownership = "application"
	OwnershipDependency  Ownership = "dependency"
	OwnershipAny         Ownership = "any"
)

type SourceLocation struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type Parameter struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type"`
}

type Result struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type"`
}

type Receiver struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Pointer bool   `json:"pointer"`
}

type GenericInfo struct {
	TypeParams []string `json:"typeParams,omitempty"`
}

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
}

func FunctionID(importPath, name string) SymbolID {
	return SymbolID(importPath + separator + name)
}

func MethodID(importPath string, recv Receiver, method string) SymbolID {
	recvType := recv.Type
	if recv.Pointer {
		recvType = pointerPrefix + recvType
	}
	return SymbolID(importPath + separator + receiverOpen + recvType + receiverClose + separator + method)
}

type ParsedSymbolID struct {
	ImportPath string
	Receiver   *Receiver
	Name       string
}

func ParseSymbolID(id SymbolID) (ParsedSymbolID, error) {
	s := string(id)
	if s == "" {
		return ParsedSymbolID{}, fmt.Errorf("empty symbol id")
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

func (s Symbol) IsMethod() bool {
	return s.Kind == SymbolMethod
}

func (s Symbol) HasContext() bool {
	return len(s.ContextIndexes) > 0
}

func (s Symbol) ReturnsError() bool {
	return len(s.ErrorIndexes) > 0
}
