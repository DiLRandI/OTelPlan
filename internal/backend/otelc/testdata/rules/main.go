package main

import (
	"context"
	"encoding/json"
	"os"

	"example.com/probe/ops"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

func main() {
	mode := "nested"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	otel.SetTracerProvider(provider)
	if mode == "noop" {
		otel.SetTracerProvider(noop.NewTracerProvider())
	}
	ctx, root := otel.Tracer("probe").Start(context.Background(), "root")
	var err error
	var recovered string
	switch mode {
	case "concurrent":
		results := make(chan error, 16)
		for range 16 {
			go func() { results <- ops.Outer(ctx) }()
		}
		for range 16 {
			result := <-results
			if result == nil || result.Error() != "probe failure" {
				panic("concurrent return changed")
			}
			err = result
		}
	case "method":
		err = (&ops.Worker{}).Execute(1, ctx)
	case "root":
		err = ops.Root()
	case "nil":
		err = ops.Outer(nil)
	case "unrecorded":
		err = ops.Unrecorded(ctx)
	case "panic":
		recovered = catchPanic(ctx)
	default:
		err = ops.Outer(ctx)
	}
	root.End()
	type item struct {
		Name, ID, Parent, Trace, Scope, Version string
		Error                                   bool
		Events                                  int
	}
	output := struct {
		Spans          []item
		ReturnedError  string
		RecoveredPanic string
	}{RecoveredPanic: recovered}
	if err != nil {
		output.ReturnedError = err.Error()
	}
	for _, span := range exporter.GetSpans() {
		output.Spans = append(output.Spans, item{span.Name, span.SpanContext.SpanID().String(), span.Parent.SpanID().String(), span.SpanContext.TraceID().String(), span.InstrumentationScope.Name, span.InstrumentationScope.Version, span.Status.Code.String() == "Error", len(span.Events)})
	}
	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		panic(err)
	}
}

func catchPanic(ctx context.Context) (value string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			value = recovered.(string)
		}
	}()
	ops.Crash(ctx)
	return ""
}
