package handlers

import (
	"github.com/go-playground/validator/v10"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
)

func registerSourceCreateFormValidation(v *validator.Validate) {
	rules := map[string]string{
		"Name":    "min=1,max=100", // name should be min 1 character and maximum 100
		"Http":    "omitnil,http_url,startsnotwith=https",
		"Https":   "omitnil,http_url,startswith=https",
		"NoProxy": "omitnil",
	}

	v.RegisterValidation("sshPublicKey", func(fl validator.FieldLevel) bool {
		return true
	})
	v.RegisterValidation("certificateChain", func(fl validator.FieldLevel) bool {
		return true
	})
	v.RegisterStructValidationMapRules(rules, v1alpha1.SourceCreate{})
}
