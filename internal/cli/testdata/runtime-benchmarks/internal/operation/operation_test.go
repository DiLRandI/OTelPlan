package operation_test

import (
	"context"
	"errors"
	"testing"

	"example.com/runtimebench/internal/operation"
)

func TestOperationsReturnCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := operation.Zero(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Zero error=%v; want context.Canceled", err)
	}

	err = operation.Two(ctx, true, 42)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Two error=%v; want context.Canceled", err)
	}

	err = operation.Fail(ctx, context.DeadlineExceeded)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Fail error=%v; want context.Canceled", err)
	}
}
