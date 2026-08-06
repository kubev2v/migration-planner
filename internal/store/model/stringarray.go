package model

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

type StringArray []string

func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	if len(a) == 0 {
		return "{}", nil
	}
	escaped := make([]string, len(a))
	for i, s := range a {
		escaped[i] = `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
	}
	return "{" + strings.Join(escaped, ",") + "}", nil
}

func (a *StringArray) Scan(src any) error {
	if src == nil {
		*a = nil
		return nil
	}
	var raw string
	switch v := src.(type) {
	case string:
		raw = v
	case []byte:
		raw = string(v)
	default:
		return fmt.Errorf("unsupported type for StringArray: %T", src)
	}

	raw = strings.TrimSpace(raw)
	if raw == "{}" || raw == "" {
		*a = StringArray{}
		return nil
	}

	raw = strings.TrimPrefix(raw, "{")
	raw = strings.TrimSuffix(raw, "}")
	*a = parsePostgresArray(raw)
	return nil
}

// TypedStringPtr converts a typed string pointer (e.g. *DeployedEnvironmentInputEnvironment) to *string.
func TypedStringPtr[T ~string](v *T) *string {
	if v == nil {
		return nil
	}
	s := string(*v)
	return &s
}

// TypedStringSlice converts a typed string slice pointer (e.g. *[]NsxInputFeatures) to StringArray.
func TypedStringSlice[T ~string](v *[]T) StringArray {
	if v == nil {
		return nil
	}
	result := make(StringArray, len(*v))
	for i, item := range *v {
		result[i] = string(item)
	}
	return result
}

// ToTypedSlice converts StringArray back to a typed string slice (e.g. []NsxInputFeatures).
func ToTypedSlice[T ~string](arr StringArray) []T {
	result := make([]T, len(arr))
	for i, s := range arr {
		result[i] = T(s)
	}
	return result
}

func parsePostgresArray(s string) []string {
	result := make([]string, 0, 1)
	var current strings.Builder
	inQuotes := false
	escaped := false

	for _, r := range s {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		switch {
		case r == '\\':
			escaped = true
		case r == '"':
			inQuotes = !inQuotes
		case r == ',' && !inQuotes:
			result = append(result, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	return append(result, current.String())
}
