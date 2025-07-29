package model

import (
	"encoding/json"

	"github.com/google/uuid"
)

type ShareToken struct {
	Token    string    `json:"token" gorm:"primaryKey;not null"`
	SourceID uuid.UUID `json:"sourceId" gorm:"not null;unique"`
	Source   Source    `json:"source" gorm:"foreignKey:SourceID"`
}

type ShareTokenList []ShareToken

func (s ShareToken) String() string {
	val, _ := json.Marshal(s)
	return string(val)
}

func NewShareToken(sourceID uuid.UUID, token string) ShareToken {
	return ShareToken{
		Token:    token,
		SourceID: sourceID,
	}
}
