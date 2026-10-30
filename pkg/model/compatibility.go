// Package model preserves compatibility names and canonical symbol-ID helpers.
// New domain types belong in internal/model. Product compatibility contracts are
// policy YAML, lockfiles, CLI JSON, and diagnostic codes.
package model

import (
	"errors"
	"fmt"
	"strings"

	internalmodel "github.com/DiLRandI/OTelPlan/internal/model"
)

var (
	errEmptySymbolID       = errors.New("empty symbol id")
	errMalformedMethodID   = errors.New("malformed method symbol id")
	errMalformedFunctionID = errors.New("malformed function symbol id")
)

const (
	receiverOpen  = "("
	receiverClose = ")"
	pointerPrefix = "*"
	separator     = "."
)

// SymbolID is the canonical identity of a function or method. Function IDs use
// import.path.Name; method IDs include the receiver form so pointer and value
// methods remain distinct.
type SymbolID = internalmodel.SymbolID

// SymbolKind distinguishes free functions from methods.
type SymbolKind = internalmodel.SymbolKind

// Symbol kinds used in the code model.
const (
	SymbolFunction = internalmodel.SymbolFunction
	SymbolMethod   = internalmodel.SymbolMethod
)

// Visibility records whether a symbol is exported from its package.
type Visibility = internalmodel.Visibility

// Visibility values recorded for discovered symbols.
const (
	VisibilityExported   = internalmodel.VisibilityExported
	VisibilityUnexported = internalmodel.VisibilityUnexported
)

// Ownership identifies whether a discovered symbol belongs to the application
// or a dependency.
type Ownership = internalmodel.Ownership

// Ownership values used by policy matching.
const (
	OwnershipApplication = internalmodel.OwnershipApplication
	OwnershipDependency  = internalmodel.OwnershipDependency
	OwnershipAny         = internalmodel.OwnershipAny
)

// SourceLocation identifies the source file and one-based line and column of a
// discovered declaration. Unknown positions use zero.
type SourceLocation = internalmodel.SourceLocation

// Parameter describes one function or method parameter by source name and Go
// type expression.
type Parameter = internalmodel.Parameter

// Result describes one function or method result by source name and Go type
// expression.
type Result = internalmodel.Result

// Receiver describes the declared receiver type, optional source name, and
// whether the declaration uses a pointer receiver.
type Receiver = internalmodel.Receiver

// GenericInfo records the type parameter names declared by a symbol.
type GenericInfo = internalmodel.GenericInfo

// Symbol is the normalized metadata used to select and explain one exact Go
// declaration, including signature, context and error result indexes.
type Symbol = internalmodel.Symbol

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
type ParsedSymbolID = internalmodel.ParsedSymbolID

// ParseSymbolID validates and splits a canonical symbol ID into its import
// path, optional receiver, and declaration name.
func ParseSymbolID(id SymbolID) (ParsedSymbolID, error) {
	symbol := string(id)
	if symbol == "" {
		return ParsedSymbolID{}, errEmptySymbolID
	}

	if idx := strings.LastIndex(symbol, receiverOpen); idx > 0 && symbol[idx-1] == separator[0] {
		closeRel := strings.Index(symbol[idx:], receiverClose)
		if closeRel < 0 {
			return ParsedSymbolID{}, fmt.Errorf("%w %q: unterminated receiver", errMalformedMethodID, symbol)
		}

		idxClose := idx + closeRel
		recvPart := symbol[idx+1 : idxClose]

		rest := symbol[idxClose+1:]
		if !strings.HasPrefix(rest, separator) || len(rest) == 1 {
			return ParsedSymbolID{}, fmt.Errorf("%w %q", errMalformedMethodID, symbol)
		}

		method := rest[1:]
		recv := &Receiver{
			Name:    "",
			Type:    strings.TrimPrefix(recvPart, pointerPrefix),
			Pointer: strings.HasPrefix(recvPart, pointerPrefix),
		}

		if recv.Type == "" {
			return ParsedSymbolID{}, fmt.Errorf("%w %q: empty receiver type", errMalformedMethodID, symbol)
		}

		return ParsedSymbolID{ImportPath: symbol[:idx-1], Receiver: recv, Name: method}, nil
	}

	idx := strings.LastIndex(symbol, separator)
	if idx <= 0 || idx == len(symbol)-1 {
		return ParsedSymbolID{}, fmt.Errorf("%w %q", errMalformedFunctionID, symbol)
	}

	return ParsedSymbolID{ImportPath: symbol[:idx], Receiver: nil, Name: symbol[idx+1:]}, nil
}

// ModuleInfo identifies a module in the loaded build and records its ownership
// and any replacement selected by the module graph.
type ModuleInfo = internalmodel.ModuleInfo

// ModuleReplacement describes the module path, version, or local directory
// that replaces a module in the effective build.
type ModuleReplacement = internalmodel.ModuleReplacement

// PackageInfo describes a discovered package and the source files that belong
// to it, including whether the package is generated or test-only.
type PackageInfo = internalmodel.PackageInfo

// InterfaceRelation records a concrete type that implements an interface and
// whether the relation is through the pointer method set.
type InterfaceRelation = internalmodel.InterfaceRelation

// CallPrecision describes callee resolution, not proof that a call executes.
type CallPrecision = internalmodel.CallPrecision

// Call precision distinguishes a known callee from a conservative candidate.
const (
	CallPrecisionStatic       = internalmodel.CallPrecisionStatic
	CallPrecisionConservative = internalmodel.CallPrecisionConservative
)

// CallGraphInfo records the algorithm, scope, and limits of advisory call analysis.
type CallGraphInfo = internalmodel.CallGraphInfo

// CallRelation records a caller-to-callee edge and whether discovery resolved
// it statically or retained conservative candidates.
type CallRelation = internalmodel.CallRelation

// InterfaceMethod maps an interface method to the concrete method selected for
// that implementation, distinguishing value and pointer concrete types.
type InterfaceMethod = internalmodel.InterfaceMethod

// TypeField describes a field in a discovered type, including whether it is
// exported or embedded.
type TypeField = internalmodel.TypeField

// TypeImport records an import used while rendering a discovered type.
type TypeImport = internalmodel.TypeImport

// TypeInfo describes a discovered type expression, its kind, element type,
// imports, and fields.
type TypeInfo = internalmodel.TypeInfo

// BuildEnvironment describes the Go inputs that can change package selection
// or type checking. Lock fingerprints use normalized manifest contents rather
// than the paths retained here for reading the effective manifests.
type BuildEnvironment = internalmodel.BuildEnvironment

// CodeModel contains normalized discovery metadata for an effective Go build.
// CallGraph and CallEdges are populated only when call analysis is requested.
type CodeModel = internalmodel.CodeModel

// Public schema and backend identifiers accepted by the policy and lockfile
// formats.
const (
	APIVersionV1Alpha1      = internalmodel.APIVersionV1Alpha1
	KindInstrumentationPlan = internalmodel.KindInstrumentationPlan
	LockAPIVersionV1Alpha1  = internalmodel.LockAPIVersionV1Alpha1
	BackendNameOTelC        = internalmodel.BackendNameOTelC
)

// ContextMode selects whether a target requires a usable context argument or
// may start a root span.
type ContextMode = internalmodel.ContextMode

// Context modes supported by policy defaults.
const (
	ContextModeRequire = internalmodel.ContextModeRequire
	ContextModeRoot    = internalmodel.ContextModeRoot
)

// ProjectConfig selects packages and build inputs for code discovery.
type ProjectConfig = internalmodel.ProjectConfig

// BackendConfig pins the backend name and version used to compile a plan.
type BackendConfig = internalmodel.BackendConfig

// ContextDefaults sets the policy-wide context requirement for targets.
type ContextDefaults = internalmodel.ContextDefaults

// ErrorDefaults sets whether selected returned errors are recorded by default.
type ErrorDefaults = internalmodel.ErrorDefaults

// CaptureDefaults controls the default argument and result capture settings.
// Safe policy defaults leave both disabled.
type CaptureDefaults = internalmodel.CaptureDefaults

// Defaults contains policy-wide span, context, error, and attribute settings.
type Defaults = internalmodel.Defaults

// Match contains the ANDed selector fields used to identify candidate symbols;
// values within one field are alternatives.
type Match = internalmodel.Match

// SafetyClassification describes the sensitivity category assigned to an
// explicitly captured attribute.
type SafetyClassification = internalmodel.SafetyClassification

// Safety classifications used by attribute validation.
const (
	ClassificationPublic   = internalmodel.ClassificationPublic
	ClassificationInternal = internalmodel.ClassificationInternal
	ClassificationPII      = internalmodel.ClassificationPII
	ClassificationSecret   = internalmodel.ClassificationSecret
)

// Safety records an attribute's classification and whether the policy permits
// that capture.
type Safety = internalmodel.Safety

// Symbol metadata fields identify stable, explicitly selected declaration facts.
const (
	SymbolFieldID          = internalmodel.SymbolFieldID
	SymbolFieldPackage     = internalmodel.SymbolFieldPackage
	SymbolFieldPackageName = internalmodel.SymbolFieldPackageName
	SymbolFieldName        = internalmodel.SymbolFieldName
	SymbolFieldReceiver    = internalmodel.SymbolFieldReceiver
	SymbolFieldSignature   = internalmodel.SymbolFieldSignature
	SymbolFieldKind        = internalmodel.SymbolFieldKind
)

// AttributeSource identifies one argument, result, constant, symbol, or error type from
// which a policy attribute is obtained.
type AttributeSource = internalmodel.AttributeSource

// AttributeRule maps an explicit telemetry key to a source and optional safety
// acknowledgement.
type AttributeRule = internalmodel.AttributeRule

// SpanConfig defines target span naming and kind overrides.
type SpanConfig = internalmodel.SpanConfig

// ErrorConfig overrides whether a rule records returned errors.
type ErrorConfig = internalmodel.ErrorConfig

// Rule combines selectors with exclusions and instrumentation behavior for its
// matching symbols.
type Rule = internalmodel.Rule

// Exclusion removes matching symbols from policy selection.
type Exclusion = internalmodel.Exclusion

// Policy is the complete backend-independent instrumentation policy supplied by
// the user.
type Policy = internalmodel.Policy

// ContextStrategy identifies the source of a span context. Index is the
// zero-based parameter index when Strategy is ContextStrategyArgument.
type ContextStrategy = internalmodel.ContextStrategy

// Context strategies distinguish parameter propagation, root spans, and no context.
const (
	ContextStrategyArgument = internalmodel.ContextStrategyArgument
	ContextStrategyRoot     = internalmodel.ContextStrategyRoot
	ContextStrategyNone     = internalmodel.ContextStrategyNone
)

// ErrorStrategy selects the zero-based result indexes whose errors are recorded.
type ErrorStrategy = internalmodel.ErrorStrategy

// AttributePlan binds an explicit telemetry attribute to its source and
// safety classification after policy resolution.
type AttributePlan = internalmodel.AttributePlan

// ResolvedTarget identifies one exact instrumentation target and the rule
// responsible for its span, context, error, and attribute behavior.
type ResolvedTarget = internalmodel.ResolvedTarget

// SkipReason explains why a candidate was not selected for instrumentation.
type SkipReason = internalmodel.SkipReason

// Skip reasons identify visibility, exclusion, context, and ownership decisions.
const (
	SkipNotExported    = internalmodel.SkipNotExported
	SkipExcluded       = internalmodel.SkipExcluded
	SkipMissingContext = internalmodel.SkipMissingContext
	SkipDependency     = internalmodel.SkipDependency
)

// SkippedTarget records a rejected candidate, its matching rule, and the reason.
type SkippedTarget = internalmodel.SkippedTarget

// ResolvedPlan contains exact selected targets and explanations for skipped
// candidates. It is the backend-independent result of policy resolution.
type ResolvedPlan = internalmodel.ResolvedPlan

// LockContext records the context strategy and parameter index frozen for a
// resolved target.
type LockContext = internalmodel.LockContext

// LockErrors records whether returned errors are captured and which result
// indexes carry them.
type LockErrors = internalmodel.LockErrors

// LockAttribute records an attribute source and its safety decision in the
// reproducible plan.
type LockAttribute = internalmodel.LockAttribute

// LockTarget freezes one exact symbol selection together with its signature,
// source rule, span behavior, safety metadata, and source location. Line and
// column changes are diagnostic metadata; file changes count as source drift.
type LockTarget = internalmodel.LockTarget

// LockBackend records backend identity and capabilities. Digest is the optional
// executable digest verified by the compilation phase.
type LockBackend = internalmodel.LockBackend

// Lockfile is the reproducible record of policy, build, backend, target, and
// generated artifact identities.
type Lockfile = internalmodel.Lockfile

// DiffClassification identifies the part of a locked plan that changed.
type DiffClassification = internalmodel.DiffClassification

// Lock diff classifications used when comparing a current resolution with a
// locked plan.
const (
	DiffAdd           = internalmodel.DiffAdd
	DiffRemove        = internalmodel.DiffRemove
	DiffSignature     = internalmodel.DiffSignature
	DiffPolicy        = internalmodel.DiffPolicy
	DiffSpanName      = internalmodel.DiffSpanName
	DiffContext       = internalmodel.DiffContext
	DiffErrorStrategy = internalmodel.DiffErrorStrategy
	DiffAttribute     = internalmodel.DiffAttribute
	DiffBackend       = internalmodel.DiffBackend
	DiffSource        = internalmodel.DiffSource
	DiffBuild         = internalmodel.DiffBuild
	DiffArtifact      = internalmodel.DiffArtifact
)

// LockDiffEntry describes one classified change, optionally tied to a symbol.
type LockDiffEntry = internalmodel.LockDiffEntry

// LockDiff is the ordered set of changes between two lock states.
type LockDiff = internalmodel.LockDiff

// Severity classifies a diagnostic as an error, warning, or informational
// message.
type Severity = internalmodel.Severity

// Diagnostic severities used by validation and compilation reports.
const (
	SeverityError   = internalmodel.SeverityError
	SeverityWarning = internalmodel.SeverityWarning
	SeverityInfo    = internalmodel.SeverityInfo
)

// Code is the stable identifier for a diagnostic category.
type Code = internalmodel.Code

// Diagnostic codes identify selector, context, safety, backend, and lockfile
// validation outcomes.
const (
	CodeInvalidSelector        = internalmodel.CodeInvalidSelector
	CodeUnresolvedSymbol       = internalmodel.CodeUnresolvedSymbol
	CodeConflictingRules       = internalmodel.CodeConflictingRules
	CodeUnknownTemplateVar     = internalmodel.CodeUnknownTemplateVar
	CodeInvalidPolicy          = internalmodel.CodeInvalidPolicy
	CodeMissingContext         = internalmodel.CodeMissingContext
	CodeMultipleContexts       = internalmodel.CodeMultipleContexts
	CodeSecretAttribute        = internalmodel.CodeSecretAttribute
	CodePIIAttribute           = internalmodel.CodePIIAttribute
	CodeCardinalityWarning     = internalmodel.CodeCardinalityWarning
	CodeBroadPlan              = internalmodel.CodeBroadPlan
	CodeCaptureNotAllowed      = internalmodel.CodeCaptureNotAllowed
	CodeRecursiveTarget        = internalmodel.CodeRecursiveTarget
	CodeManualSpan             = internalmodel.CodeManualSpan
	CodeBackendUnsupported     = internalmodel.CodeBackendUnsupported
	CodeBackendVersionMismatch = internalmodel.CodeBackendVersionMismatch
	CodeCompilationFailed      = internalmodel.CodeCompilationFailed
	CodeArtifactOutput         = internalmodel.CodeArtifactOutput
	CodeStaleLockfile          = internalmodel.CodeStaleLockfile
)

// DiagnosticError describes one explainable problem or notice, with optional policy
// rule, symbol, and source location context.
type DiagnosticError = internalmodel.DiagnosticError

// DiagnosticErrorList is an ordered collection of diagnostics produced by a model
// operation.
type DiagnosticErrorList = internalmodel.DiagnosticErrorList

// BackendCapabilities describes the instrumentation operations a backend supports.
type BackendCapabilities = internalmodel.BackendCapabilities

// Artifacts groups generated files under their output directory.
type Artifacts = internalmodel.Artifacts

// ArtifactFile identifies a generated file by path and content digest.
type ArtifactFile = internalmodel.ArtifactFile

// BuildRequest supplies a resolved plan, generated artifacts, Go arguments,
// and source and working directories to a backend build.
type BuildRequest = internalmodel.BuildRequest

// Backend is the contract for validating plans, generating artifacts, and
// executing instrumented builds. Implementations report their capabilities
// separately so unsupported targets can be diagnosed before compilation.
type Backend = internalmodel.Backend
