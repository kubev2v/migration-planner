package validator

import (
	"github.com/go-playground/validator/v10"
)

type ValidationRule struct {
	Rule func(v *validator.Validate)
}

// Validator is a wrapper around the actual validator
// It sets up the validator and extract the rule error message from the underlying error
type Validator struct {
	validator *validator.Validate
	rules     []ValidationRule
}

func NewValidator() *Validator {
	v := validator.New()
	return &Validator{validator: v}
}

func (v *Validator) Register(rules ...ValidationRule) {
	for _, validationRule := range rules {
		validationRule.Rule(v.validator)
	}
	v.rules = rules
}

func (v *Validator) Struct(s any) error {
	return v.validator.Struct(s)
}
