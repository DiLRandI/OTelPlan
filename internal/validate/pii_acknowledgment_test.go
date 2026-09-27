package validate_test

import (
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/validate"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestPIIAcknowledgment(t *testing.T) {
	t.Parallel()

	code, symbol := attributeFixture(t)

	cases := []struct {
		name           string
		key            string
		classification model.SafetyClassification
		allow          bool
		warning        bool
	}{
		{name: "unclassified email", key: "email", classification: "", allow: false, warning: true},
		{name: "public acknowledgment", key: "email", classification: model.ClassificationPublic, allow: true, warning: true},
		{name: "PII acknowledgment", key: "email", classification: model.ClassificationPII, allow: true, warning: false},
		{name: "secret acknowledgment", key: "email", classification: model.ClassificationSecret,
			allow: true, warning: false},
		{name: "explicit PII", key: "kind", classification: model.ClassificationPII, allow: false, warning: true},
		{name: "secret only", key: "kind", classification: model.ClassificationSecret, allow: false, warning: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var target model.ResolvedTarget

			target.SymbolID = symbol.ID
			target.Attributes = []model.AttributePlan{{
				Key:            testCase.key,
				From:           model.AttributeSource{Argument: "", Result: "", Constant: "private"},
				Classification: testCase.classification,
				Allow:          testCase.allow,
			}}

			var plan model.ResolvedPlan

			plan.Targets = []model.ResolvedTarget{target}

			var options validate.Options

			warning := false

			for _, diagnostic := range validate.Safety(code, plan, options) {
				if diagnostic.Code == model.CodePIIAttribute {
					warning = true
				}
			}

			if warning != testCase.warning {
				t.Fatalf("PII warning = %v, want %v", warning, testCase.warning)
			}
		})
	}
}
