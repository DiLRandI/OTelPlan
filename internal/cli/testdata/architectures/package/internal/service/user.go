package service

import (
	"context"

	"example.com/architecture/internal/probe"
)

type User struct{}

func (u User) Authorize(ctx context.Context, token string) error {
	u.Health(ctx)
	return probe.Reject(ctx, token)
}
func (User) Health(context.Context) {}
