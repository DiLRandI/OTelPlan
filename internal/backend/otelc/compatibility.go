package otelc

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const SupportedVersion = "v1.1.0"

const (
	variadicBoolType   = "bool"
	variadicStringType = "string"
)

func Identity(version string) (model.LockBackend, error) {
	if version != SupportedVersion {
		return model.LockBackend{}, fmt.Errorf("unsupported otelc version; expected %s", SupportedVersion)
	}

	return model.LockBackend{Name: model.BackendNameOTelC, Version: version, Capabilities: model.BackendCapabilities{BeforeHook: true, AfterHook: true, ArgumentRead: true, ArgumentReplace: true, ResultRead: true, ContextReplacement: true, FunctionEntrySelection: true}}, nil
}

func Check(version string, code *model.CodeModel, plan model.ResolvedPlan) model.DiagnosticErrorList {
	if _, err := Identity(version); err != nil {
		return model.DiagnosticErrorList{{Severity: model.SeverityError, Code: model.CodeBackendVersionMismatch, Message: err.Error()}}
	}

	if code == nil {
		return model.DiagnosticErrorList{{Severity: model.SeverityError, Code: model.CodeBackendUnsupported, Message: "backend requires an analyzed code model"}}
	}

	var diags model.DiagnosticErrorList

	for _, target := range plan.Targets {
		add := func(message string) {
			diags = append(diags, model.DiagnosticError{Severity: model.SeverityError, Code: model.CodeBackendUnsupported, RuleID: target.RuleID, Symbol: target.SymbolID, Message: message})
		}

		symbol, ok := code.Symbol(target.SymbolID)

		if !ok || symbol.Signature != target.Signature {
			add("backend target must match the analyzed symbol and signature")

			continue
		}

		if symbol.PackageName == "main" {
			add("main package targets require verified command-specific build scoping")
		}

		if symbol.Variadic {
			if _, supported := variadicElementType(symbol); !supported {
				add("variadic targets require a built-in element type that generated hooks can name safely")
			}
		}

		if !symbol.HasBody {
			add("backend cannot hook a declaration without a Go body")
		}

		if hasTypeParameters(symbol) && requiresGenericValueAPIs(target) {
			add("otelc v1.1.0 cannot replace generic context arguments or capture argument/result attributes; " +
				"see upstream issue 1280")
		}

		switch target.ContextStrategy.Strategy {
		case model.ContextStrategyArgument:
			index := target.ContextStrategy.Index
			if len(symbol.ContextIndexes) != 1 || symbol.ContextIndexes[0] != index || index < 0 || index >= len(symbol.Parameters) {
				add("context replacement requires the unique analyzed context argument")
			}
		case model.ContextStrategyRoot:
		default:
			add("unsupported context strategy")
		}

		for _, index := range target.ErrorStrategy.Indexes {
			known := false

			for _, candidate := range symbol.ErrorIndexes {
				if candidate == index {
					known = true
				}
			}

			if !target.ErrorStrategy.Record || !known || index < 0 || index >= len(symbol.Results) {
				add("error strategy refers to an invalid error result")
			}
		}
	}

	return diags
}

func hasTypeParameters(symbol *model.Symbol) bool {
	return symbol.Generics != nil && len(symbol.Generics.TypeParams) > 0
}

func requiresGenericValueAPIs(target model.ResolvedTarget) bool {
	if target.ContextStrategy.Strategy != model.ContextStrategyRoot {
		return true
	}

	for _, attribute := range target.Attributes {
		if attribute.From.Argument != "" || attribute.From.Result != "" {
			return true
		}
	}

	return false
}

func variadicElementType(symbol *model.Symbol) (string, bool) {
	if !symbol.Variadic || len(symbol.Parameters) == 0 {
		return "", false
	}

	element, ok := strings.CutPrefix(symbol.Parameters[len(symbol.Parameters)-1].Type, "[]")
	if !ok {
		return "", false
	}

	switch element {
	case "any", variadicBoolType, "byte", "complex64", "complex128", "error", "float32", "float64",
		"int", "int8", "int16", "int32", "int64", "interface{}", "rune", variadicStringType, "uint",
		"uint8", "uint16", "uint32", "uint64", "uintptr":
		return element, true
	default:
		return "", false
	}
}

func VerifyExecutable(ctx context.Context, executable, version string) (model.LockBackend, error) {
	identity, err := Identity(version)
	if err != nil {
		return identity, err
	}

	path, err := exec.LookPath(executable)
	if err != nil {
		return identity, fmt.Errorf("find pinned otelc executable: %w", err)
	}

	command := exec.CommandContext(ctx, path, "version")

	output, err := command.Output()
	if err != nil {
		return identity, fmt.Errorf("read otelc version: %w", err)
	}

	fields := strings.Fields(string(output))
	if len(fields) < 3 || fields[0] != "otelc" || fields[1] != "version" {
		return identity, errors.New("unrecognized otelc version output")
	}

	reported, _, _ := strings.Cut(fields[2], "+")
	if reported != version {
		return identity, errors.New("otelc executable does not match pinned version")
	}

	file, err := os.Open(path)
	if err != nil {
		return identity, fmt.Errorf("open otelc executable: %w", err)
	}

	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return identity, fmt.Errorf("digest otelc executable: %w", err)
	}

	identity.Digest = fmt.Sprintf("sha256:%x", hash.Sum(nil))

	return identity, nil
}
