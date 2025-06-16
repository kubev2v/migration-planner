package validator

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"regexp"

	"github.com/go-playground/validator/v10"
)

var (
	sshRegex = []*regexp.Regexp{
		regexp.MustCompile("^ssh-rsa AAAAB3NzaC1yc2[0-9A-Za-z+/]+[=]{0,3}(\\s.*)?$"),
		regexp.MustCompile("^ssh-ed25519 AAAAC3NzaC1lZDI1NTE5[0-9A-Za-z+/]+[=]{0,3}(\\s.*)?$"),
		regexp.MustCompile("^ssh-dss AAAAB3NzaC1kc3[0-9A-Za-z+/]+[=]{0,3}(\\s.*)?$"),
		regexp.MustCompile("^ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNT[0-9A-Za-z+/]+[=]{0,3}(\\s.*)?$"),
	}

	sourceNameValidRegex = regexp.MustCompile("^[a-zA-Z0-9+-_.]+$")
)

func nameValidator(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Interface().(string)
	if !ok {
		return false
	}

	return sourceNameValidRegex.MatchString(val)
}

func sshKeyValidator(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Addr().Interface().(*string)
	if !ok {
		return false
	}

	for _, r := range sshRegex {
		if r.MatchString(*val) {
			return true
		}
	}

	return false
}

func certificateValidator(fl validator.FieldLevel) bool {
	val, ok := fl.Field().Addr().Interface().(*string)
	if !ok {
		return false
	}

	block, _ := pem.Decode(bytes.NewBufferString(*val).Bytes())
	if block == nil {
		return false
	}

	_, err := x509.ParseCertificate(block.Bytes)
	return err == nil
}
