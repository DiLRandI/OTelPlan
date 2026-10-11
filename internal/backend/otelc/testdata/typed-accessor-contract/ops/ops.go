package ops

import (
	"context"
	"math"
	"example.com/accessorprobe/dep"
)

type InnerChild struct {
	Value float64
}

type Inner struct {
	Score float64
	Child *InnerChild
}

type request struct {
	ID      string
	Enabled bool
	Count   uint64
	Signed  int64
	Code    dep.Code
	Secret  string
	Inner   *Inner
	secret  string
}

type output struct {
	Message string
	Big     uint64
	Score   float64
	NaN     float64
}

func NewRequest() *request {
	return &request{
		ID:      "",
		Enabled: false,
		Count:   9223372036854775808,
		Code:    dep.Code("named"),
		Secret:  "hidden",
		Inner:   &Inner{Score: math.Inf(1), Child: nil},
	}
}

func NewResult() output {
	return output{Message: "", Big: 9223372036854775808, Score: math.Inf(1), NaN: math.NaN()}
}

func Handle(ctx context.Context, req *request, code dep.Code) (result output, err error) {
	return output{}, nil
}
