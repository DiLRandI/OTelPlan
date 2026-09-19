package probe

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
)

func Reject(ctx context.Context, _ string) error {
	_, span := otel.Tracer("fixture").Start(ctx, "downstream")
	defer span.End()
	return errors.New("declined")
}
