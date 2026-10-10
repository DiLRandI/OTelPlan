package otelc

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

// SupportedVersion pins the verified OTelC backend release.
const SupportedVersion = "v1.1.0"

const (
	variadicBoolType   = "bool"
	variadicStringType = "string"
)

var (
	errUnsupportedOTelCVersion   = errors.New("unsupported otelc version; expected " + SupportedVersion)
	errUnrecognizedVersion       = errors.New("unrecognized otelc version output")
	errExecutableVersionMismatch = errors.New("otelc executable does not match pinned version")
)

// Identity returns the capabilities verified for the pinned backend version.
// Executable verification supplies its digest separately.
func Identity(version string) (model.LockBackend, error) {
	var identity model.LockBackend

	if version != SupportedVersion {
		return identity, errUnsupportedOTelCVersion
	}

	identity.Name = model.BackendNameOTelC
	identity.Version = version
	identity.Capabilities = model.BackendCapabilities{
		BeforeHook: true, AfterHook: true, ArgumentRead: true, ArgumentReplace: true, ResultRead: true,
		PanicObservation: false, ContextReplacement: true, FunctionEntrySelection: true, FunctionCallSelection: false,
	}

	return identity, nil
}

// Check reports unsupported targets in plan order, preserving policy provenance.
func Check(version string, code *model.CodeModel, plan model.ResolvedPlan) model.DiagnosticErrorList {
	_, err := Identity(version)
	if err != nil {
		return model.DiagnosticErrorList{backendCompatibilityDiagnostic(model.CodeBackendVersionMismatch, err.Error())}
	}

	if code == nil {
		return model.DiagnosticErrorList{backendCompatibilityDiagnostic(model.CodeBackendUnsupported,
			"backend requires an analyzed code model")}
	}

	var diagnostics model.DiagnosticErrorList

	for _, target := range plan.Targets {
		symbol, exists := code.Symbol(target.SymbolID)

		var issues []string

		if !exists || symbol.Signature != target.Signature {
			issues = []string{"backend target must match the analyzed symbol and signature"}
		} else {
			issues = symbolCompatibilityIssues(code, symbol, target)
			issues = append(issues, contextCompatibilityIssues(symbol, target.ContextStrategy)...)
			issues = append(issues, errorCompatibilityIssues(symbol, target.ErrorStrategy)...)
		}

		for _, message := range issues {
			diagnostic := backendCompatibilityDiagnostic(model.CodeBackendUnsupported, message)
			diagnostic.RuleID, diagnostic.Symbol = target.RuleID, target.SymbolID
			diagnostics = append(diagnostics, diagnostic)
		}
	}

	return diagnostics
}

func backendCompatibilityDiagnostic(code model.Code, message string) model.DiagnosticError {
	var diagnostic model.DiagnosticError

	diagnostic.Severity = model.SeverityError
	diagnostic.Code = code
	diagnostic.Message = message

	return diagnostic
}

func symbolCompatibilityIssues(code *model.CodeModel, symbol *model.Symbol,
	target model.ResolvedTarget) []string {
	var issues []string

	if symbol.PackageName == "main" {
		issues = append(issues, "main package targets require verified command-specific build scoping")
	}

	if symbol.Variadic {
		_, supported := variadicElementType(symbol)
		if !supported {
			issues = append(issues, "variadic targets require a built-in element type that generated hooks can name safely")
		}
	}

	if !symbol.HasBody {
		issues = append(issues, "backend cannot hook a declaration without a Go body")
	}

	if hasTypeParameters(symbol) {
		if target.ContextStrategy.Strategy != model.ContextStrategyRoot {
			issues = append(issues, "otelc v1.1.0 cannot replace generic context arguments; see upstream issue 1280")
		}

		issue := genericCaptureIssue(code, symbol, target.Attributes)
		if issue != "" {
			issues = append(issues, issue)
		}
	}

	return issues
}

func contextCompatibilityIssues(symbol *model.Symbol, strategy model.ContextStrategy) []string {
	switch strategy.Strategy {
	case model.ContextStrategyArgument:
		index := strategy.Index
		if len(symbol.ContextIndexes) != 1 || symbol.ContextIndexes[0] != index ||
			index < 0 || index >= len(symbol.Parameters) {
			return []string{"context replacement requires the unique analyzed context argument"}
		}
	case model.ContextStrategyRoot:
	default:
		return []string{"unsupported context strategy"}
	}

	return nil
}

func errorCompatibilityIssues(symbol *model.Symbol, strategy model.ErrorStrategy) []string {
	var issues []string

	for _, index := range strategy.Indexes {
		if !strategy.Record || !slices.Contains(symbol.ErrorIndexes, index) || index < 0 || index >= len(symbol.Results) {
			issues = append(issues, "error strategy refers to an invalid error result")
		}
	}

	return issues
}

func hasTypeParameters(symbol *model.Symbol) bool {
	return symbol.Generics != nil && len(symbol.Generics.TypeParams) > 0
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

// VerifyExecutable checks the selected executable version and returns its content digest.
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

	err = validateReportedVersion(output, version)
	if err != nil {
		return identity, err
	}

	identity.Digest, err = executableDigest(path)
	if err != nil {
		return identity, err
	}

	return identity, nil
}

func validateReportedVersion(output []byte, version string) error {
	fields := strings.Fields(string(output))
	if len(fields) < 3 || fields[0] != "otelc" || fields[1] != "version" {
		return errUnrecognizedVersion
	}

	reported, _, _ := strings.Cut(fields[2], "+")
	if reported != version {
		return errExecutableVersionMismatch
	}

	return nil
}

func executableDigest(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve otelc executable: %w", err)
	}

	root, err := os.OpenRoot(filepath.Dir(resolved))
	if err != nil {
		return "", fmt.Errorf("open otelc executable directory: %w", err)
	}

	digest, readErr := hashExecutable(root, filepath.Base(resolved))

	closeErr := root.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close otelc executable directory: %w", closeErr)
	}

	if readErr != nil || closeErr != nil {
		return "", errors.Join(readErr, closeErr)
	}

	return digest, nil
}

func hashExecutable(root *os.Root, name string) (string, error) {
	file, err := root.Open(name)
	if err != nil {
		return "", fmt.Errorf("open otelc executable: %w", err)
	}

	hash := sha256.New()
	_, readErr := io.Copy(hash, file)

	closeErr := file.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close otelc executable: %w", closeErr)
	}

	if readErr != nil || closeErr != nil {
		return "", fmt.Errorf("digest otelc executable: %w", errors.Join(readErr, closeErr))
	}

	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}
