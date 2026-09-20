package adapters

import (
	"context"

	"example.com/architecture/internal/probe"
)

type Gateway struct{}

func (g *Gateway) Authorize(ctx context.Context, token string) error {
	g.Health(ctx)
	return probe.Reject(ctx, token)
}
func (*Gateway) Health(context.Context) {}
