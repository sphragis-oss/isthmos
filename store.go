// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Store struct {
	dir string
	key []byte
	ttl time.Duration
}

// OpenStore prepares the reversibility store, creating dir and key on first use
func OpenStore(dir string, ttl time.Duration) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(dir, "store.key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.WriteFile(keyPath, key, 0o600); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("store key must be 32 bytes")
	}
	return &Store{dir: dir, key: key, ttl: ttl}, nil
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

func validID(id string) bool {
	if len(id) != 16 {
		return false
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func (s *Store) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Save seals the payload under id and opportunistically drops expired entries
func (s *Store) Save(id string, payload []byte) error {
	if !validID(id) {
		return errors.New("invalid store id")
	}
	g, err := s.gcm()
	if err != nil {
		return err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	sealed := g.Seal(nonce, nonce, payload, []byte(id))
	if err := os.WriteFile(filepath.Join(s.dir, id+".bin"), sealed, 0o600); err != nil {
		return err
	}
	s.gc()
	return nil
}

// Load opens the sealed payload for id
func (s *Store) Load(id string) ([]byte, error) {
	if !validID(id) {
		return nil, errors.New("invalid store id")
	}
	sealed, err := os.ReadFile(filepath.Join(s.dir, id+".bin"))
	if err != nil {
		return nil, err
	}
	g, err := s.gcm()
	if err != nil {
		return nil, err
	}
	if len(sealed) < g.NonceSize() {
		return nil, errors.New("corrupt store entry")
	}
	return g.Open(nil, sealed[:g.NonceSize()], sealed[g.NonceSize():], []byte(id))
}

// gc removes entries older than ttl, best effort
func (s *Store) gc() {
	if s.ttl <= 0 {
		return
	}
	cutoff := time.Now().Add(-s.ttl)
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".bin" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(s.dir, e.Name()))
		}
	}
}
