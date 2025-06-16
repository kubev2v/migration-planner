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

func NewSourceValidationRules() []ValidationRule {
	return []ValidationRule{
		{
			Tag:                    "name",
			Rule:                   registerAlias("name", "source_name,min=1,max=100"),
			ValidationErrorMessage: "source name should have between 1 and 100 chars",
		},
		{
			Tag:                    "httpUrl",
			Rule:                   registerAlias("httpUrl", "url,startsnotwith=https"),
			ValidationErrorMessage: "http proxy url must be a valid url and not starting with https",
		},
		{
			Tag:                    "httpsUrl",
			Rule:                   registerAlias("httpsUrl", "url,startswith=https"),
			ValidationErrorMessage: "https proxy url must be a valid url and it should start with https",
		},
		{
			Tag:                    "noProxy",
			Rule:                   registerAlias("noProxy", "max=1000"),
			ValidationErrorMessage: "noProxy should have maximum 1000 characters",
		},
		{
			Tag:                    "sshPublicKey",
			Rule:                   registerAlias("sshPublicKey", "omitnil,ssh_key"),
			ValidationErrorMessage: "invalid ssh key",
		},
		{
			Tag:                    "certificateChain",
			Rule:                   registerAlias("certificateChain", "omitnil,certs"),
			ValidationErrorMessage: "invalid certificate chain",
		},
		{
			Tag:                    "proxy",
			Rule:                   registerAlias("proxy", "omitnil"),
			ValidationErrorMessage: "invalid proxy definition",
		},
		{
			Tag:                    "ssh_key",
			Rule:                   registerFn("ssh_key", sshKeyValidator),
			ValidationErrorMessage: "invalid ssh key",
		},
		{
			Tag:                    "source_name",
			Rule:                   registerFn("source_name", nameValidator),
			ValidationErrorMessage: "source name contains invalid characters",
		},
		{
			Tag:                    "certs",
			Rule:                   registerFn("certs", certificateValidator),
			ValidationErrorMessage: "invalid certificate chain",
		},
	}
}

func registerAlias(tag, rule string) func(v *validator.Validate) {
	return func(v *validator.Validate) {
		v.RegisterAlias(tag, rule)
	}
}

func registerFn(tag string, fn func(fl validator.FieldLevel) bool) func(v *validator.Validate) {
	return func(v *validator.Validate) {
		v.RegisterValidation(tag, fn)
	}
}
