package compiler_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/compiler"
)

func TestReadModuleSelectionFailure(t *testing.T) {
	t.Parallel()

	env := append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOPROXY=off")

	modules, err := compiler.ReadModuleSelection(t.Context(), t.TempDir(), env)
	if err == nil || modules != nil {
		t.Fatal("accepted directory without module")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	modules, err = compiler.ReadModuleSelection(ctx, t.TempDir(), env)
	if !errors.Is(err, context.Canceled) || modules != nil {
		t.Fatalf("cancellation not preserved: %v", err)
	}
}
