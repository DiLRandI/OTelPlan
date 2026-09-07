package main

import (
	"context"
	"encoding/json"
	"example.com/probe/ops"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"os"
)

func main() {
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	otel.SetTracerProvider(provider)
	ctx, root := provider.Tracer("probe").Start(context.Background(), "root")
	_ = ops.Outer(ctx)
	root.End()
	type item struct {
		Name   string
		ID     string
		Parent string
		Trace  string
		Error  bool
		Events int
	}
	var output []item
	for _, span := range exporter.GetSpans() {
		output = append(output, item{span.Name, span.SpanContext.SpanID().String(), span.Parent.SpanID().String(), span.SpanContext.TraceID().String(), span.Status.Code.String() == "Error", len(span.Events)})
	}
	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		panic(err)
	}
}
