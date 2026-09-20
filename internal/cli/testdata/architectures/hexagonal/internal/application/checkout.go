package application

import (
	"context"

	"example.com/architecture/internal/ports"
)

type Checkout struct{ Payments ports.Payment }

func (c Checkout) Submit(ctx context.Context, token string) error {
	return c.Payments.Authorize(ctx, token)
}
