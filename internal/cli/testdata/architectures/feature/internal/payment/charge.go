package payment

import (
	"context"

	"example.com/architecture/internal/probe"
)

func Authorize(ctx context.Context, token string) error { Health(ctx); return probe.Reject(ctx, token) }
func Health(context.Context)                            {}
