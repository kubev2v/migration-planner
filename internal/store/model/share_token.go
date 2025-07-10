package model

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ShareToken struct {
	gorm.Model
	ID       uuid.UUID `json:"id" gorm:"primaryKey"`
	Token    string    `json:"token" gorm:"not null;unique"`
	SourceID uuid.UUID `json:"sourceId" gorm:"not null"`
	Source   Source    `json:"source" gorm:"foreignKey:SourceID"`
}

type ShareTokenList []ShareToken

func (s ShareToken) String() string {
	val, _ := json.Marshal(s)
	return string(val)
}

func NewShareToken(sourceID uuid.UUID, token string) ShareToken {
	return ShareToken{
		ID:       uuid.New(),
		Token:    token,
		SourceID: sourceID,
	}
}
