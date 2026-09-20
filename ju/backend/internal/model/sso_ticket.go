package model

import "time"

// SSOTicket stores only the digest of a consumed one-time identifier.
type SSOTicket struct {
	ID        string    `gorm:"primaryKey;size:64"`
	ExpiresAt time.Time `gorm:"index"`
}

type SSOHandoff struct {
	ID        string    `gorm:"primaryKey;size:64"`
	Payload   string    `gorm:"type:text"`
	ExpiresAt time.Time `gorm:"index"`
}

type SSORevocation struct {
	Subject  string `gorm:"primaryKey;size:160"`
	IssuedAt int64
}
