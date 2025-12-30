package util

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
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

// Contains checks if a slice contains a specific string
func Contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// ConvertBytesToMB converts bytes to megabytes safely
func ConvertBytesToMB(bytes int64) int64 {
	return bytes / (1024 * 1024)
}

// ConvertMBToBytes converts megabytes to bytes safely
func ConvertMBToBytes(mb int64) int64 {
	return mb * 1024 * 1024
}

// DerefString safely dereferences a string pointer, returning an empty string if the pointer is nil
func DerefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ToStrPtr returns a pointer to the given string
func ToStrPtr(s string) *string {
	return &s
}

// IntPtr returns a pointer to the given int
func IntPtr(i int) *int {
	return &i
}

// FloatPtr returns a pointer to the given float
func FloatPtr(i float64) *float64 {
	return &i
}

// Round Method to round to 2 decimals
func Round(f float64) float64 {
	return math.Round(f*100) / 100
}

// BytesToTB converts a value in bytes to terabytes (TB).
// Accepts int, int64, or float64.
func BytesToTB[T ~int | ~int64 | ~float64](bytes T) float64 {
	return float64(bytes) / 1024.0 / 1024.0 / 1024.0 / 1024.0
}

// BytesToGB converts a value in bytes to gigabytes (GB).
// Accepts int, int64, or float64.
func BytesToGB[T ~int | ~int64 | ~float64](bytes T) int {
	return int(math.Round(float64(bytes) / 1024.0 / 1024.0 / 1024.0))
}

// GBToTB converts a value in gigabytes (GB) to terabytes (TB).
// Accepts int, int64, or float64.
func GBToTB[T ~int | ~int64 | ~float64](gb T) float64 {
	return float64(gb) / 1024.0
}

// MBToGB converts a value in MB to GB.
// Accepts int, int32, or float64.
func MBToGB[T ~int | ~int32 | ~float64](mb T) int {
	return int(math.Round(float64(mb) / 1024.0))
}

// BoolPtr returns a pointer to the given bool
func BoolPtr(b bool) *bool {
	return &b
}

// Unmarshal does not return error when v1 inventory is unmarshal into a v2 struct.
// The only way to differentiate the version is to check the internal structure.
func GetInventoryVersion(inventory []byte) int {
	i := v1alpha1.Inventory{}
	_ = json.Unmarshal(inventory, &i)

	if i.VcenterId == "" {
		return model.SnapshotVersionV1
	}
	return model.SnapshotVersionV2
}
