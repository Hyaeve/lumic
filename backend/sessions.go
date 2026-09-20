package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

const rememberedSessionLifetime = 7 * 24 * time.Hour

type rememberedSession struct {
	Expires     time.Time `json:"expires"`
	Credentials string    `json:"credentials"`
}

func sessionDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *SessionStore) credentialsDigest() string {
	if s.auth == nil {
		return ""
	}
	s.auth.RLock()
	defer s.auth.RUnlock()
	return sessionDigest(s.auth.Username + "\x00" + s.auth.PasswordHash)
}

// All sessions are process-local, including remembered browser sessions.
// Never restore old sessions.json files after a container restart.
func newSessionStore(auth *AuthConfig) *SessionStore {
	return &SessionStore{tokens: make(map[string]time.Time), remembered: make(map[string]rememberedSession), auth: auth}
}

func (s *SessionStore) createRemembered() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	s.Lock()
	defer s.Unlock()
	if s.remembered == nil {
		s.remembered = make(map[string]rememberedSession)
	}
	credentials := s.credentialsDigest()
	for key, record := range s.remembered {
		if !record.Expires.After(time.Now()) || record.Credentials != credentials {
			delete(s.remembered, key)
		}
	}
	key := sessionDigest(token)
	s.remembered[key] = rememberedSession{time.Now().Add(rememberedSessionLifetime), credentials}
	return token, nil
}
