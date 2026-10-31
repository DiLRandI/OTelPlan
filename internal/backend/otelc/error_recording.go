package otelc

const errorRecordingGuardSource = `
span.SetStatus(codes.Error, "operation failed")
func() {
 defer func() {
  if recover() != nil {
   span.AddEvent("exception", trace.WithAttributes(attribute.String("exception.message", "error message unavailable")))
  }
 }()
 span.RecordError(err)
}()
`
