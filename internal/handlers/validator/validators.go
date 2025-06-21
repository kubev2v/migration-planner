package validator

import (
	"errors"

	"github.com/go-playground/validator/v10"
)

type ValidationRule struct {
	Tag                    string
	Rule                   func(v *validator.Validate)
	ValidationErrorMessage string
}

// Validator is a wrapper around the actual validator
// It sets up the validator and extract the rule error message from the underlying error
type Validator struct {
	validator *validator.Validate
	rules     []ValidationRule
}

func NewValidator() *Validator {
	v := validator.New()
	v.SetTagName("json")
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

func (v *Validator) GetErrorMessage(err error) []string {
	getRule := func(tag string) *ValidationRule {
		for _, r := range v.rules {
			if tag == r.Tag {
				return &r
			}
		}
		return nil
	}

	errMessages := []string{}
	var validationError validator.ValidationErrors
	if errors.As(err, &validationError) {
		for _, fieldError := range validationError {
			for _, tag := range []string{fieldError.Tag(), fieldError.ActualTag()} {
				rule := getRule(tag)
				if rule != nil {
					errMessages = append(errMessages, rule.ValidationErrorMessage)
					continue
				}
			}
		}
		return errMessages
	}

	return []string{err.Error()}
}

func registerAlias(tag, rule string) func(v *validator.Validate) {
	return func(v *validator.Validate) {
		v.RegisterAlias(tag, rule)
	}
}

func registerFn(tag string, fn func(fl validator.FieldLevel) bool) func(v *validator.Validate) {
	return func(v *validator.Validate) {
		_ = v.RegisterValidation(tag, fn)
	}
}
