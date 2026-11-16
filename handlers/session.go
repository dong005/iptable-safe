package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const (
	AdminSessionTTL   = 1 * time.Hour
	AdminCookieName   = "admin_token"
	MinPasswordLength = 6
)

type adminSession struct {
	IP        string
	ExpiresAt time.Time
}

var (
	sessionsMu sync.RWMutex
	sessions   = make(map[string]adminSession)
)

func generateSecureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func createAdminSession(clientIP string) (string, error) {
	token, err := generateSecureToken()
	if err != nil {
		return "", err
	}
	sessionsMu.Lock()
	sessions[token] = adminSession{
		IP:        clientIP,
		ExpiresAt: time.Now().Add(AdminSessionTTL),
	}
	sessionsMu.Unlock()
	return token, nil
}

func validateAdminSession(token string) bool {
	if token == "" {
		return false
	}
	sessionsMu.RLock()
	s, ok := sessions[token]
	sessionsMu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(s.ExpiresAt) {
		revokeAdminSession(token)
		return false
	}
	return true
}

func revokeAdminSession(token string) {
	sessionsMu.Lock()
	delete(sessions, token)
	sessionsMu.Unlock()
}

func revokeAllAdminSessions() {
	sessionsMu.Lock()
	sessions = make(map[string]adminSession)
	sessionsMu.Unlock()
}

func CleanupExpiredSessions() {
	now := time.Now()
	sessionsMu.Lock()
	for token, s := range sessions {
		if now.After(s.ExpiresAt) {
			delete(sessions, token)
		}
	}
	sessionsMu.Unlock()
}
