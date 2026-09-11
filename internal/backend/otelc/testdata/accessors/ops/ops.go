package ops

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
)

type request struct {
	ID     string
	Secret string
}

func NewRequest(id, secret string) *request {
	return &request{ID: id, Secret: secret}
}

func Handle(ctx context.Context, request *request) error {
	_, child := otel.Tracer("probe").Start(ctx, "downstream")
	child.End()
	if request == nil {
		return nil
	}
	return errors.New("operation failed")
}
