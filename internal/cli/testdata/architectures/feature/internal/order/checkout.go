package order

import (
	"context"

	"example.com/architecture/internal/payment"
)

func Submit(ctx context.Context, token string) error { return payment.Authorize(ctx, token) }
func Health(context.Context)                         {}
