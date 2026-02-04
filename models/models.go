package models

import (
	"time"
)

type WhitelistIP struct {
	ID          int       `json:"id"`
	IP          string    `json:"ip"`
	Description string    `json:"description"`
	IsPermanent bool      `json:"is_permanent"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type Config struct {
	ID            int    `json:"id"`
	UserPassword  string `json:"user_password"`
	AdminPassword string `json:"admin_password"`
}

type LoginAttempt struct {
	IP        string    `json:"ip"`
	Timestamp time.Time `json:"timestamp"`
	Success   bool      `json:"success"`
}
