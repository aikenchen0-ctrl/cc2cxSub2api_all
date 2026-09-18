package model

// SSOTicket records consumed ticket digests across restarts and replicas.
type SSOTicket struct {
	Digest    string `gorm:"primaryKey;size:64"`
	ExpiresAt int64  `gorm:"index"`
}
