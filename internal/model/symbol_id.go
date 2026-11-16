package model

const (
	receiverOpen  = "("
	receiverClose = ")"
	pointerPrefix = "*"
	separator     = "."
)

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
