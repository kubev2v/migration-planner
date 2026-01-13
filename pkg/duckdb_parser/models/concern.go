package models

// Concern represents a validation concern for a VM.
type Concern struct {
	Id         string `json:"id" db:"Concern_ID"`
	Label      string `json:"label" db:"Label"`
	Category   string `json:"category" db:"Category"`
	Assessment string `json:"assessment" db:"Assessment"`
}
