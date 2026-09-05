package model

import "testing"

func TestFunctionIDRoundTrip(t *testing.T) {
	id := FunctionID("github.com/acme/shop/internal/payment", "ProcessPayment")
	parsed, err := ParseSymbolID(id)
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
	cases := []struct {
		name    string
		recv    Receiver
		method  string
		wantID  SymbolID
	}{
		{
			name:   "value receiver",
			recv:   Receiver{Name: "p", Type: "Processor"},
			method: "Authorize",
			wantID: "github.com/acme/shop/internal/payment.(Processor).Authorize",
		},
		{
			name:   "pointer receiver",
			recv:   Receiver{Name: "p", Type: "Processor", Pointer: true},
			method: "Authorize",
			wantID: "github.com/acme/shop/internal/payment.(*Processor).Authorize",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
id := MethodID("github.com/acme/shop/internal/payment", tc.recv, tc.method)
if id != tc.wantID {
t.Fatalf("id = %q, want %q", id, tc.wantID)
}
parsed, err := ParseSymbolID(id)
if err != nil {
t.Fatalf("parse: %v", err)
}
if parsed.Receiver == nil {
t.Fatal("receiver = nil, want non-nil")
}
if parsed.Receiver.Type != "Processor" {
t.Errorf("receiver type = %q", parsed.Receiver.Type)
}
if parsed.Receiver.Pointer != tc.recv.Pointer {
t.Errorf("receiver pointer = %v, want %v", parsed.Receiver.Pointer, tc.recv.Pointer)
}
if parsed.Name != tc.method {
t.Errorf("method = %q", parsed.Name)
}
})
	}
}

func TestParseSymbolIDRejectsMalformed(t *testing.T) {
	for _, id := range []SymbolID{"", "no-separator", "pkg.()", "pkg.().method", "pkg.(Recv)."} {
		if _, err := ParseSymbolID(id); err == nil {
			t.Errorf("ParseSymbolID(%q) = nil error, want error", id)
		}
	}
}

func TestSymbolPredicates(t *testing.T) {
	s := Symbol{ContextIndexes: []int{0}, ErrorIndexes: []int{0}}
	if !s.HasContext() || !s.ReturnsError() {
		t.Error("predicates should be true")
	}
	empty := Symbol{}
	if empty.HasContext() || empty.ReturnsError() {
		t.Error("predicates should be false")
	}
}
