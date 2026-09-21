package model_test

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestFunctionIDRoundTrip(t *testing.T) {
	t.Parallel()

	id := model.FunctionID("github.com/acme/shop/internal/payment", "ProcessPayment")

	parsed, err := model.ParseSymbolID(id)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if parsed.ImportPath != "github.com/acme/shop/internal/payment" {
		t.Errorf("import path = %q", parsed.ImportPath)
	}

	if parsed.Name != "ProcessPayment" {
		t.Errorf("name = %q", parsed.Name)
	}

	if parsed.Receiver != nil {
		t.Errorf("receiver = %v, want nil", parsed.Receiver)
	}
}

func TestMethodIDRoundTrip(t *testing.T) {
	t.Parallel()

	const receiverType = "Processor"

	cases := []struct {
		name   string
		recv   model.Receiver
		method string
		wantID model.SymbolID
	}{
		{
			name:   "value receiver",
			recv:   model.Receiver{Name: "p", Type: receiverType, Pointer: false},
			method: "Authorize",
			wantID: "github.com/acme/shop/internal/payment.(Processor).Authorize",
		},
		{
			name:   "pointer receiver",
			recv:   model.Receiver{Name: "p", Type: receiverType, Pointer: true},
			method: "Authorize",
			wantID: "github.com/acme/shop/internal/payment.(*Processor).Authorize",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			id := model.MethodID("github.com/acme/shop/internal/payment", testCase.recv, testCase.method)
			if id != testCase.wantID {
				t.Fatalf("id = %q, want %q", id, testCase.wantID)
			}

			parsed, err := model.ParseSymbolID(id)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			if parsed.Receiver == nil {
				t.Fatal("receiver = nil, want non-nil")
			}

			if parsed.Receiver.Type != receiverType {
				t.Errorf("receiver type = %q", parsed.Receiver.Type)
			}

			if parsed.Receiver.Pointer != testCase.recv.Pointer {
				t.Errorf("receiver pointer = %v, want %v", parsed.Receiver.Pointer, testCase.recv.Pointer)
			}

			if parsed.Name != testCase.method {
				t.Errorf("method = %q", parsed.Name)
			}
		})
	}
}

func TestParseSymbolIDRejectsMalformed(t *testing.T) {
	t.Parallel()

	for _, id := range []model.SymbolID{"", "no-separator", "pkg.()", "pkg.().method", "pkg.(Recv)."} {
		_, err := model.ParseSymbolID(id)
		if err == nil {
			t.Errorf("ParseSymbolID(%q) = nil error, want error", id)
		}
	}
}

func TestSymbolPredicates(t *testing.T) {
	t.Parallel()

	var symbol model.Symbol

	symbol.ContextIndexes = []int{0}
	symbol.ErrorIndexes = []int{0}

	if !symbol.HasContext() || !symbol.ReturnsError() {
		t.Error("predicates should be true")
	}

	var empty model.Symbol
	if empty.HasContext() || empty.ReturnsError() {
		t.Error("predicates should be false")
	}
}

func TestParseSymbolIDErrorMessages(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id      model.SymbolID
		message string
	}{
		{id: "", message: "empty symbol id"},
		{id: "pkg.(*Worker", message: `malformed method symbol id "pkg.(*Worker": unterminated receiver`},
		{id: "pkg.(Worker)", message: `malformed method symbol id "pkg.(Worker)"`},
		{id: "pkg.().Run", message: `malformed method symbol id "pkg.().Run": empty receiver type`},
		{id: "no-separator", message: `malformed function symbol id "no-separator"`},
	}
	for _, testCase := range cases {
		t.Run(string(testCase.id), func(t *testing.T) {
			t.Parallel()

			parsed, err := model.ParseSymbolID(testCase.id)
			if err == nil || err.Error() != testCase.message {
				t.Fatalf("error = %v, want %q", err, testCase.message)
			}

			var empty model.ParsedSymbolID

			if parsed != empty {
				t.Fatalf("parsed = %+v, want zero value", parsed)
			}
		})
	}
}
