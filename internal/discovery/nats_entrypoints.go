package discovery

import (
	"context"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"
)

const (
	natsImportPath      = "github.com/nats-io/nats.go"
	jetstreamImportPath = natsImportPath + "/jetstream"
)

func natsEntrypoints(_ context.Context, info *types.Info, call *ast.CallExpr) []entrypointCandidate {
	registrar, _ := typeutil.Callee(info, call).(*types.Func)

	path, name := natsCallbackType(registrar)
	if path == "" {
		return nil
	}

	signature := registrar.Signature()
	for parameter := range signature.Params().Variables() {
		if !natsCallbackParameter(parameter.Type(), path, name) {
			continue
		}

		argument := forwardedEntrypointArgument(signature, parameter, info, call)
		if argument == nil {
			return nil
		}

		handler := entrypointFunction(info, entrypointExpression(info, argument))
		if handler == nil {
			return nil
		}

		return []entrypointCandidate{{function: handler, integration: string(functionSymbolID(registrar))}}
	}

	return nil
}

func natsCallbackType(function *types.Func) (string, string) {
	if function == nil || function.Pkg() == nil || function.Signature().Recv() == nil {
		return "", ""
	}

	switch function.Pkg().Path() {
	case natsImportPath:
		if function.Name() == "Subscribe" || function.Name() == "QueueSubscribe" {
			return natsImportPath, "MsgHandler"
		}
	case jetstreamImportPath:
		if function.Name() == "Consume" {
			return jetstreamImportPath, "MessageHandler"
		}
	}

	return "", ""
}

func natsCallbackParameter(typ types.Type, path, name string) bool {
	named, ok := types.Unalias(typ).(*types.Named)

	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == path && named.Obj().Name() == name
}
