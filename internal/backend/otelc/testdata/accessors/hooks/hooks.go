package hooks

import (
	"context"

	"example.com/probe/ops"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otelc/pkg/hook"
)

func BeforeHandle(h hook.HookContext, _ any, _ any) { before(h, "handle-request") }
func AfterHandle(h hook.HookContext, _ any)         { after(h) }

func before(h hook.HookContext, name string) {
	ctx, _ := h.GetParam(0).(context.Context)
	if ctx == nil {
		ctx = context.Background()
	}
	child, span := otel.Tracer("otelplan.io/business").Start(ctx, name, trace.WithSpanKind(trace.SpanKindInternal))
	if id, ok := ops.RequestID(h.GetParam(1)); ok {
		span.SetAttributes(attribute.String("request.id", id))
	}
	h.SetParam(0, child)
	h.SetData(span)
}

func after(h hook.HookContext) {
	span, ok := h.GetData().(trace.Span)
	if !ok {
		return
	}
	defer span.End()
	if err, ok := h.GetReturnVal(0).(error); ok && err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "operation failed")
	}
}
