package entry

import (
	"context"

	"example.com/architecture/internal/order"
)

func Run(ctx context.Context, token string) error { return order.Submit(ctx, token) }
