package cli

import (
	"context"
	"errors"
	"io/fs"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/DiLRandI/OTelPlan/internal/backend/otelc"
	"github.com/DiLRandI/OTelPlan/internal/policy"
	"github.com/DiLRandI/OTelPlan/pkg/model"
	"golang.org/x/mod/semver"
)

const versionProbeTimeout = 5 * time.Second

func versionCommand(ctx context.Context, opts options, args []string) (any, int, model.DiagnosticErrorList) {
	if len(args) != 0 {
		return nil, exitUsage, compileDiagnostic(model.CodeInvalidPolicy, "version takes no positional arguments")
	}

	versions := map[string]string{"otelplan": Version, "go": runtime.Version(), "backend": model.BackendNameOTelC,
		"backendSupportedVersion": otelc.SupportedVersion, "backendStatus": "unverified"}

	err := ctx.Err()
	if err != nil {
		return versionProbeFailure(opts, versions, err)
	}

	diagnostics := configuredBackendVersion(opts, versions)
	if diagnostics.HasErrors() {
		return versions, exitInvalidPolicy, diagnostics
	}

	executable, err := exec.LookPath("otelc")
	if errors.Is(err, exec.ErrNotFound) {
		versions["backendStatus"] = "unavailable"

		return versions, 0, nil
	}

	if err != nil {
		return versionProbeFailure(opts, versions, err)
	}

	probe, cancel := context.WithTimeout(ctx, versionProbeTimeout)
	defer cancel()

	version, err := otelc.ExecutableVersion(probe, executable)
	if err != nil {
		return versionProbeFailure(opts, versions, err)
	}

	versions["backendStatus"], versions["backendAvailableVersion"] = "available", version

	return versions, 0, nil
}

func configuredBackendVersion(opts options, versions map[string]string) model.DiagnosticErrorList {
	config := opts.config
	if !filepath.IsAbs(config) {
		config = filepath.Join(opts.root, config)
	}

	loaded, err := policy.Load(config)
	if errors.Is(err, fs.ErrNotExist) && !opts.configSet {
		return nil
	}

	if err != nil {
		recordFailure(opts, model.CodeInvalidPolicy, "read configured backend", err)

		return compileDiagnostic(model.CodeInvalidPolicy, "cannot read or parse policy for backend version details")
	}

	backend := loaded.Backend

	exact, _, _ := strings.Cut(backend.Version, "+")
	if backend.Name != model.BackendNameOTelC || !semver.IsValid(backend.Version) ||
		semver.Canonical(backend.Version) != exact ||
		len(backend.Version) > 128 {
		return compileDiagnostic(model.CodeInvalidPolicy,
			"backend version details require the otelc backend and an exact version")
	}

	versions["backendConfiguredVersion"] = semver.Canonical(backend.Version)

	return nil
}

func versionProbeFailure(opts options, versions map[string]string, cause error) (any, int, model.DiagnosticErrorList) {
	versions["backendStatus"] = "unverified"

	recordFailure(opts, model.CodeBackendUnsupported, "query backend version", cause)

	return versions, exitBackend, compileDiagnostic(model.CodeBackendUnsupported,
		"cannot query the available otelc version")
}
