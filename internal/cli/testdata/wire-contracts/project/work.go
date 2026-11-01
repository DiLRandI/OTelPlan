package wire

import "context"

type Worker struct{}

type Category string

func (c Category) String() string { return string(c) }

type Request struct {
	Kind Category
}

type Result struct {
	OK bool
}

func (*Worker) Run(req Request, ctx context.Context) (result Result, err error) {
	return Result{OK: req.Kind != ""}, ctx.Err()
}

func Check(ctx context.Context) error { return ctx.Err() }

func Skip(context.Context) error { return nil }

func localSkip(context.Context) error { return nil }
