package ops

import "math"

type captureRequest struct {
	ID     string
	Count  uint64
	Secret string
}

type captureResult struct {
	Message   string
	Empty     string
	Big       uint64
	NonFinite float64
}

func NewCaptureRequest() *captureRequest {
	return &captureRequest{ID: "approved-id", Count: math.MaxUint64, Secret: "private-request-marker"}
}

func Capture[T any](value T, req *captureRequest, enabled bool) (result captureResult, err error) {
	return newCaptureResult(), nil
}

func (Store[T]) Capture(req *captureRequest, enabled bool) (value T, result captureResult, err error) {
	return value, newCaptureResult(), nil
}

func (Store[T]) UnsupportedCapture(req *captureRequest) (captureResult, error) {
	return newCaptureResult(), nil
}

func newCaptureResult() captureResult {
	return captureResult{Message: "approved-output", Empty: "", Big: math.MaxUint64, NonFinite: math.NaN()}
}

type parametricRequest[T any] struct {
	ID    string
	Value T
}

func Parametric[T any](req *parametricRequest[T]) error {
	return nil
}
