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

type Worker struct{}

func (*Worker) Handle(ctx context.Context, request *request) (size int, err error) {
	_, child := otel.Tracer("probe").Start(ctx, "downstream")
	child.End()
	if request == nil {
		return 0, nil
	}
	return len(request.ID), errors.New("operation failed")
}
