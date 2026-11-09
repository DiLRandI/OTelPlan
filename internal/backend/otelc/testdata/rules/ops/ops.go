package ops

import (
	"context"
	"errors"
)

func Outer(ctx context.Context) error { return Inner(ctx) }
func Inner(ctx context.Context) error { return errors.New("probe failure") }

type Worker struct{}

func (*Worker) Execute(_ int, ctx context.Context) error { return Inner(ctx) }
func PreserveNil(ctx context.Context) error {
	if ctx == nil {
		return nil
	}

	return errors.New("preserve nil received non-nil context")
}
func (*Worker) PreserveNil(_ int, ctx context.Context) error { return PreserveNil(ctx) }
func Root() error                                            { return nil }
func Unrecorded(context.Context) error                       { return errors.New("unrecorded failure") }
func Crash(context.Context)                                  { panic("application panic") }
func CrashIfNil(ctx context.Context) {
	if ctx == nil {
		panic("nil context panic")
	}
}
