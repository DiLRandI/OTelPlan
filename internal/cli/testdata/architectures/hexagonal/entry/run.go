package entry

import (
	"context"

	"example.com/architecture/internal/adapters"
	"example.com/architecture/internal/application"
)

func Run(ctx context.Context, token string) error {
	flow := application.Checkout{Payments: &adapters.Gateway{}}
	return flow.Submit(ctx, token)
}
