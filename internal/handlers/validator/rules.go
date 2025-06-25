package validator

import "github.com/go-playground/validator/v10"

func registerFn(tag string, fn func(fl validator.FieldLevel) bool) func(v *validator.Validate) {
	return func(v *validator.Validate) {
		_ = v.RegisterValidation(tag, fn)
	}
}

func NewAgentValidationRules() []ValidationRule {
	return []ValidationRule{
		{
			Rule: registerFn("sourceId", uuidValidator),
		},
		{
			Rule: registerFn("status", agentStatusValidator),
		},
	}
}

func NewSourceValidationRules() []ValidationRule {
	return []ValidationRule{
		{
			Rule: registerFn("ssh_key", sshKeyValidator),
		},
		{
			Rule: registerFn("source_name", nameValidator),
		},
		{
			Rule: registerFn("certs", certificateValidator),
		},
	}
}
