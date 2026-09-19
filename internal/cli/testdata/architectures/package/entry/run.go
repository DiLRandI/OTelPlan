package entry

import (
	"context"

	"example.com/architecture/internal/service"
)

func Run(ctx context.Context, token string) error { return (&service.Order{}).Submit(ctx, token) }
