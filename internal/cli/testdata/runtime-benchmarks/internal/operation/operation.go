// Package operation contains the benchmark business operations.
package operation

import (
	"context"
	"errors"
	"fmt"
)

var errUnexpectedArguments = errors.New("unexpected benchmark arguments")

var errContext = errors.New("operation context error")

// Zero is the smallest business operation used by the runtime benchmark.
//
//go:noinline
func Zero(ctx context.Context) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("%w: %w", errContext, err)
	}

	return nil
}

// Two exercises primitive argument attributes.
//
//go:noinline
func Two(ctx context.Context, enabled bool, count int64) error {
	if !enabled || count != 42 {
		return errUnexpectedArguments
	}

	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("%w: %w", errContext, err)
	}

	return nil
}

// Fail preserves the supplied failure unless the context is canceled.
//
//go:noinline
func Fail(ctx context.Context, failure error) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("%w: %w", errContext, err)
	}

	return failure
}
