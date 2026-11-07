package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

const (
	sessionLifetime          = 30 * 24 * time.Hour
	temporarySessionLifetime = 24 * time.Hour
)

var errInvalidCredentials = errors.New("invalid credentials")

type SessionStore struct {
	sync.RWMutex
	auth    *AuthConfig
	file    string
	records map[string]sessionRecord
}

type sessionRecord struct {
	Expires     time.Time `json:"expires"`
	Credentials string    `json:"credentials"`
	persistent  bool
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

func newSessionStore(auth *AuthConfig) *SessionStore {
	return &SessionStore{records: make(map[string]sessionRecord), auth: auth}
}

func loadSessionStore(auth *AuthConfig, path string) (*SessionStore, error) {
	store := newSessionStore(auth)
	store.file = path
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	var saved map[string]sessionRecord
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, err
	}
	now, credentials := time.Now(), store.credentialsDigest()
	for digest, record := range saved {
		if record.Expires.After(now) && record.Credentials == credentials {
			record.persistent = true
			store.records[digest] = record
		}
	}
	return store, nil
}

func (s *SessionStore) login(username, password string, persistent bool) (string, time.Time, error) {
	s.auth.RLock()
	storedUsername, storedHash := s.auth.Username, s.auth.PasswordHash
	s.auth.RUnlock()
	if subtle.ConstantTimeCompare([]byte(username), []byte(storedUsername)) != 1 || !passwordMatches(password, storedHash) {
		return "", time.Time{}, errInvalidCredentials
	}
	return s.create(sessionDigest(storedUsername+"\x00"+storedHash), persistent)
}

func (s *SessionStore) create(credentials string, persistent bool) (string, time.Time, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	s.Lock()
	defer s.Unlock()
	if credentials != s.credentialsDigest() {
		return "", time.Time{}, errInvalidCredentials
	}
	now := time.Now()
	for key, record := range s.records {
		if !record.Expires.After(now) || record.Credentials != credentials {
			delete(s.records, key)
		}
	}
	lifetime := temporarySessionLifetime
	if persistent {
		lifetime = sessionLifetime
	}
	expires := now.UTC().Add(lifetime)
	key := sessionDigest(token)
	s.records[key] = sessionRecord{Expires: expires, Credentials: credentials, persistent: persistent}
	if persistent {
		if err := s.saveLocked(); err != nil {
			delete(s.records, key)
			return "", time.Time{}, err
		}
	}
	return token, expires, nil
}

func (s *SessionStore) saveLocked() error {
	saved := make(map[string]sessionRecord)
	now := time.Now()
	for key, record := range s.records {
		if record.persistent && record.Expires.After(now) {
			saved[key] = record
		}
	}
	data, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	// Never delete the last durable session snapshot if replacing it fails.
	return writeFileViaRename(s.file, data, 0600, os.Rename)
}

func (s *SessionStore) expiration(token string) (time.Time, bool) {
	s.RLock()
	defer s.RUnlock()
	record, exists := s.records[sessionDigest(token)]
	if !exists || !record.Expires.After(time.Now()) || record.Credentials != s.credentialsDigest() {
		return time.Time{}, false
	}
	return record.Expires, true
}

func (s *SessionStore) valid(token string) bool {
	_, ok := s.expiration(token)
	return ok
}

func (s *SessionStore) revoke(token string) error {
	s.Lock()
	defer s.Unlock()
	key := sessionDigest(token)
	record, exists := s.records[key]
	if !exists {
		return nil
	}
	delete(s.records, key)
	if record.persistent {
		if err := s.saveLocked(); err != nil {
			s.records[key] = record
			return err
		}
	}
	return nil
}
