package util

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

type StringerWithError func() (string, error)

func Must(err error) {
	if err != nil {
		panic(fmt.Errorf("internal error: %w", err))
	}
}

func MustString(fn StringerWithError) string {
	s, err := fn()
	if err != nil {
		panic(fmt.Errorf("internal error: %w", err))
	}
	return s
}

type Duration struct {
	time.Duration
}

func (duration *Duration) UnmarshalJSON(b []byte) error {
	var unmarshalledJson interface{}

	err := json.Unmarshal(b, &unmarshalledJson)
	if err != nil {
		return err
	}

	switch value := unmarshalledJson.(type) {
	case float64:
		duration.Duration = time.Duration(value)
	case string:
		duration.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid duration: %#v", unmarshalledJson)
	}

	return nil
}

func GetEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists && len(value) > 0 {
		return value
	}
	return defaultValue
}

func GetIntEnv(key string, defaultValue uint) (uint, error) {
	if value, exists := os.LookupEnv(key); exists && len(value) > 0 {
		u64, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return 0, err
		}
		return uint(u64), nil
	}
	return defaultValue, nil
}
