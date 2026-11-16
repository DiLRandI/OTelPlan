package model_test

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/model"
)

func BenchmarkDiagnosticHasErrors(b *testing.B) {
	for _, testCase := range []struct {
		name     string
		severity model.Severity
		want     bool
	}{
		{name: "all-warnings", severity: model.SeverityWarning, want: false},
		{name: "all-errors", severity: model.SeverityError, want: true},
	} {
		b.Run(testCase.name, func(b *testing.B) {
			diagnostics := make(model.DiagnosticErrorList, 1000)
			for index := range diagnostics {
				diagnostics[index].Severity = testCase.severity
			}

			if got := diagnostics.HasErrors(); got != testCase.want {
				b.Fatalf("HasErrors() = %t; want %t", got, testCase.want)
			}

			b.ReportAllocs()
			b.ResetTimer()

			var got bool
			for range b.N {
				got = diagnostics.HasErrors()
			}

			if got != testCase.want {
				b.Fatalf("HasErrors() = %t; want %t", got, testCase.want)
			}
		})
	}
}
