package model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ImageInfra struct {
	gorm.Model
	SourceID         uuid.UUID `gorm:"primaryKey"`
	HttpProxyUrl     string
	HttpsProxyUrl    string
	NoProxyDomains   string
	CertificateChain string
	SshPublicKey     string
	ImageTokenKey    string
	IpAddress        string
	SubnetMask       string
	DefaultGateway   string
	Dns              string
}
