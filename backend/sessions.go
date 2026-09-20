package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const rememberedSessionLifetime = 30 * 24 * time.Hour

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

func loadSessionStore(path string, auth *AuthConfig) (*SessionStore, error) {
	s := &SessionStore{tokens: make(map[string]time.Time), remembered: make(map[string]rememberedSession), sessionFile: path, auth: auth}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &s.remembered); err != nil {
		return nil, err
	}
	return s, nil
}

// Only token digests are persisted; the browser alone retains the bearer secret.
func (s *SessionStore) saveRemembered() error {
	if s.sessionFile == "" {
		return nil
	}
	data, err := json.Marshal(s.remembered)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(s.sessionFile), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(s.sessionFile), ".sessions-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), s.sessionFile)
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
	if err := s.saveRemembered(); err != nil {
		delete(s.remembered, key)
		return "", err
	}
	return token, nil
}
