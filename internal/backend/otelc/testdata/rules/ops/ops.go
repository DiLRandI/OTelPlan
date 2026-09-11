package ops

import (
	"context"
	"errors"
)

func Outer(ctx context.Context) error { return Inner(ctx) }
func Inner(ctx context.Context) error { return errors.New("probe failure") }

type Worker struct{}

func (*Worker) Execute(_ int, ctx context.Context) error { return Inner(ctx) }
func Root() error                                        { return nil }
func Unrecorded(context.Context) error                   { return errors.New("unrecorded failure") }
func Crash(context.Context)                              { panic("application panic") }
