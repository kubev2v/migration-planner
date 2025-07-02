package model

type User struct {
	Username     string `gorm:"primaryKey;column:username;type:VARCHAR;size:256"`
	FirstName    string `gorm:"column:first_name;type:VARCHAR;size:255"`
	LastName     string `gorm:"column:last_name;type:VARCHAR;size:255"`
	Organization string `gorm:"column:organization;type:VARCHAR;size:255"`
}
