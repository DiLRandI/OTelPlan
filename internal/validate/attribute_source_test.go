package validate_test

import "github.com/DiLRandI/OTelPlan/pkg/model"

func sourceAttribute(source model.AttributeSource) model.AttributePlan {
	var attribute model.AttributePlan

	attribute.From = source

	return attribute
}
