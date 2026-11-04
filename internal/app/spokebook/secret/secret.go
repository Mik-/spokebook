// Package secret seals credentials of ride sources with a key from the
// configuration (APP_SECRET_KEY), so a copy of the database alone doesn't
// reveal them.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
)

// MinKeyLen is the minimum length of the configured key.
const MinKeyLen = 32

var (
	// ErrShortKey means the configured key is shorter than MinKeyLen.
	ErrShortKey = errors.New("secret: key too short")
	// ErrOpen means the sealed data is damaged or was sealed with another
	// key or for another purpose.
	ErrOpen = errors.New("secret: cannot open")
)

// Box seals and opens data with AES-256-GCM.
type Box struct{ aead cipher.AEAD }

// New derives the encryption key from key.
func New(key string) (*Box, error) {
	if len(key) < MinKeyLen {
		return nil, ErrShortKey
	}
	k, err := hkdf.Key(sha256.New, []byte(key), nil, "spokebook credentials", 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plain. The purpose (e.g. owner and source) is bound to the
// result, so it can't be opened for another one.
func (b *Box) Seal(plain []byte, purpose string) []byte {
	nonce := make([]byte, b.aead.NonceSize(), b.aead.NonceSize()+len(plain)+b.aead.Overhead())
	_, _ = rand.Read(nonce)
	return b.aead.Seal(nonce, nonce, plain, []byte(purpose))
}

// Open decrypts what Seal returned for the same purpose.
func (b *Box) Open(sealed []byte, purpose string) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return nil, ErrOpen
	}
	plain, err := b.aead.Open(nil, sealed[:n], sealed[n:], []byte(purpose))
	if err != nil {
		return nil, ErrOpen
	}
	return plain, nil
}
