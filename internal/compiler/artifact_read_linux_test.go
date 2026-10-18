package compiler_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

const (
	artifactFIFOProbeEnvironment = "OTELPLAN_TEST_ARTIFACT_FIFO"
	artifactFIFOProbeTimeout     = 10 * time.Second
)

func TestReadArtifactsRejectsFIFO(t *testing.T) {
	t.Parallel()

	if directory := os.Getenv(artifactFIFOProbeEnvironment); directory != "" {
		verifyArtifactFIFORejection(t, directory)

		return
	}

	directory := t.TempDir()
	manifest := filepath.Join(directory, "manifest.json")

	err := syscall.Mkfifo(manifest, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), artifactFIFOProbeTimeout)
	defer cancel()

	// A separate process bounds a regression that blocks while opening the pipe.
	command := exec.CommandContext(ctx, "/proc/self/exe", "-test.run=^TestReadArtifactsRejectsFIFO$")

	command.Env = append(os.Environ(), artifactFIFOProbeEnvironment+"="+directory)

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("reject FIFO manifest without waiting for a writer: %v\n%s", err, output)
	}

	info, err := os.Lstat(manifest)
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatal("artifact verification changed the FIFO manifest", err)
	}
}

func verifyArtifactFIFORejection(t *testing.T, directory string) {
	t.Helper()

	artifacts := model.Artifacts{
		Dir: directory,
		Files: []model.ArtifactFile{{
			Path: "manifest.json", Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(nil)),
		}},
	}

	regularFileError := compiler.VerifyArtifacts(artifacts)
	if regularFileError == nil {
		t.Fatal("artifact inventory accepted a FIFO")
	}

	for {
		cause := errors.Unwrap(regularFileError)
		if cause == nil {
			break
		}

		regularFileError = cause
	}

	_, err := compiler.ReadArtifacts(directory)
	if !errors.Is(err, regularFileError) {
		t.Fatalf("manifest did not preserve non-regular artifact rejection: %v", err)
	}
}
