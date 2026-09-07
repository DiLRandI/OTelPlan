package hooks

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otelc/pkg/hook"
)

func BeforeOuter(h hook.HookContext, _ any) { before(h, "outer") }
func AfterOuter(h hook.HookContext, _ any)  { after(h) }
func BeforeInner(h hook.HookContext, _ any) { before(h, "inner") }
func AfterInner(h hook.HookContext, _ any)  { after(h) }
func before(h hook.HookContext, name string) {
	ctx, _ := h.GetParam(0).(context.Context)
	if ctx == nil {
		ctx = context.Background()
	}
	child, span := otel.Tracer("otelplan.io/business").Start(ctx, name, trace.WithSpanKind(trace.SpanKindInternal))
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
