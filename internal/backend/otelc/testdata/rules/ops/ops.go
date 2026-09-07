package ops

import (
	"context"
	"errors"
)

func Outer(ctx context.Context) error { return Inner(ctx) }
func Inner(ctx context.Context) error { return errors.New("probe failure") }
