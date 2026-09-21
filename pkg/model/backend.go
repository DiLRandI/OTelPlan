// Package model defines policy, discovery, resolution, backend, and lockfile
// data shared by OTelPlan components.
package model

import "context"

// BackendCapabilities describes the instrumentation operations a backend supports.
type BackendCapabilities struct {
	BeforeHook             bool `json:"beforeHook"`
	AfterHook              bool `json:"afterHook"`
	ArgumentRead           bool `json:"argumentRead"`
	ArgumentReplace        bool `json:"argumentReplace"`
	ResultRead             bool `json:"resultRead"`
	PanicObservation       bool `json:"panicObservation"`
	ContextReplacement     bool `json:"contextReplacement"`
	FunctionEntrySelection bool `json:"functionEntrySelection"`
	FunctionCallSelection  bool `json:"functionCallSelection"`
}

// Artifacts groups generated files under their output directory.
type Artifacts struct {
	Dir   string         `json:"dir"`
	Files []ArtifactFile `json:"files"`
}

// ArtifactFile identifies a generated file by path and content digest.
type ArtifactFile struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

// BuildRequest supplies a resolved plan, generated artifacts, Go arguments,
// and source and working directories to a backend build.
type BuildRequest struct {
	Plan      ResolvedPlan
	Artifacts Artifacts
	GoArgs    []string
	RootDir   string
	WorkDir   string
}

// Backend is the contract for validating plans, generating artifacts, and
// executing instrumented builds. Implementations report their capabilities
// separately so unsupported targets can be diagnosed before compilation.
type Backend interface {
	Name() string
	Version(ctx context.Context) (string, error)
	Capabilities(ctx context.Context) (BackendCapabilities, error)
	Validate(ctx context.Context, plan ResolvedPlan) DiagnosticErrorList
	Compile(ctx context.Context, plan ResolvedPlan, outDir string) (Artifacts, error)
	Build(ctx context.Context, req BuildRequest) error
}
