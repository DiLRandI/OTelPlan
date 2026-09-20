package service

import "context"

type Order struct{}

func (*Order) Submit(ctx context.Context, token string) error { return (User{}).Authorize(ctx, token) }
