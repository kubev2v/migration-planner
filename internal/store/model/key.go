package model

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type Key struct {
	gorm.Model
	OrgID      string          `gorm:"primaryKey"`
	ID         string          `gorm:"not null;unique"`
	PrivateKey *rsa.PrivateKey `gorm:"type:text;not null;serializer:key_serializer"`
}

type KeySerializer struct{}

func (ks KeySerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue interface{}) (err error) {
	switch value := dbValue.(type) {
	case string:
		block, _ := pem.Decode([]byte(value))
		if block == nil {
			return fmt.Errorf("unsupported data: %s", value)
		}
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return err
		}
		field.ReflectValueOf(ctx, dst).Set(reflect.ValueOf(key))
	default:
		return fmt.Errorf("unsupported data %#v", dbValue)
	}
	return nil
}

func (ks KeySerializer) Value(ctx context.Context, field *schema.Field, dst reflect.Value, fieldValue interface{}) (interface{}, error) {
	switch v := fieldValue.(type) {
	case *rsa.PrivateKey:
		pemdata := pem.EncodeToMemory(
			&pem.Block{
				Type:  "RSA PRIVATE KEY",
				Bytes: x509.MarshalPKCS1PrivateKey(v),
			},
		)
		return string(pemdata), nil
	default:
		return "", fmt.Errorf("unsupported data %v", fieldValue)
	}
}
