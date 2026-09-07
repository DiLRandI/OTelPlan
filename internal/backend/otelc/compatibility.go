package otelc

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const SupportedVersion = "v1.1.0"

func Identity(version string) (model.LockBackend, error) {
	if version != SupportedVersion {
		return model.LockBackend{}, fmt.Errorf("unsupported otelc version; expected %s", SupportedVersion)
	}
	return model.LockBackend{Name: model.BackendNameOTelC, Version: version, Capabilities: model.BackendCapabilities{BeforeHook: true, AfterHook: true, ArgumentRead: true, ArgumentReplace: true, ResultRead: true, ContextReplacement: true, FunctionEntrySelection: true}}, nil
}

func Check(version string, code *model.CodeModel, plan model.ResolvedPlan) model.DiagnosticList {
	if _, err := Identity(version); err != nil {
		return model.DiagnosticList{{Severity: model.SeverityError, Code: model.CodeBackendVersionMismatch, Message: err.Error()}}
	}
	if code == nil {
		return model.DiagnosticList{{Severity: model.SeverityError, Code: model.CodeBackendUnsupported, Message: "backend requires an analyzed code model"}}
	}
	var diags model.DiagnosticList
	for _, target := range plan.Targets {
		add := func(message string) {
			diags = append(diags, model.Diagnostic{Severity: model.SeverityError, Code: model.CodeBackendUnsupported, RuleID: target.RuleID, Symbol: target.SymbolID, Message: message})
		}
		symbol, ok := code.Symbol(target.SymbolID)
		if !ok || symbol.Signature != target.Signature {
			add("backend target must match the analyzed symbol and signature")
			continue
		}
		if !symbol.HasBody {
			add("backend cannot hook a declaration without a Go body")
		}
		if symbol.Generics != nil && len(symbol.Generics.TypeParams) > 0 {
			add("otelc v1.1.0 disables hook parameter/result APIs for generic targets; see upstream issue 1280")
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
		return identity, fmt.Errorf("unrecognized otelc version output")
	}
	reported, _, _ := strings.Cut(fields[2], "+")
	if reported != version {
		return identity, fmt.Errorf("otelc executable does not match pinned version")
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
