package main

import (
	"context"
	"encoding/json"
	"os"

	"example.com/probe/ops"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func main() {
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	otel.SetTracerProvider(provider)
	ctx, root := provider.Tracer("probe").Start(context.Background(), "root")
	worker := &ops.Worker{}
	size, err := worker.Handle(ctx, ops.NewRequest("approved-id", "top-secret"))
	errors := []string{errorText(err)}
	sizes := []int{size}
	size, err = worker.Handle(ctx, nil)
	errors = append(errors, errorText(err))
	sizes = append(sizes, size)
	root.End()
	type span struct {
		Name       string
		ID         string
		Parent     string
		Trace      string
		Error      bool
		Attributes map[string]any
		Events     int
	}
	spans := make([]span, 0, len(exporter.GetSpans()))
	for _, item := range exporter.GetSpans() {
		attributes := map[string]any{}
		for _, attribute := range item.Attributes {
			attributes[string(attribute.Key)] = attribute.Value.AsInterface()
		}
		spans = append(spans, span{Name: item.Name, ID: item.SpanContext.SpanID().String(), Parent: item.Parent.SpanID().String(), Trace: item.SpanContext.TraceID().String(), Error: item.Status.Code.String() == "Error", Attributes: attributes, Events: len(item.Events)})
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		Errors []string
		Sizes  []int
		Spans  []span
	}{errors, sizes, spans}); err != nil {
		panic(err)
	}
}

func errorText(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}
