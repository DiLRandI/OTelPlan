package compiler

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestReadModuleSelectionFailure(t *testing.T) {
	env := append(os.Environ(), "GOWORK=off", "GOFLAGS=", "GOPROXY=off")
	if modules, err := ReadModuleSelection(t.Context(), t.TempDir(), env); err == nil || modules != nil {
		t.Fatal("accepted directory without module")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if modules, err := ReadModuleSelection(ctx, t.TempDir(), env); !errors.Is(err, context.Canceled) || modules != nil {
		t.Fatalf("cancellation not preserved: %v", err)
	}
}
