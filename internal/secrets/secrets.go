// Package secrets seals presentable secrets (Instance tokens, database passwords)
// with a key derived from PLATFORM_SECRET_KEY.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
)

// version leads every sealed value so the key can be rotated later.
const version byte = 1

type Box struct{ aead cipher.AEAD }

func NewBox(secretKey string) (*Box, error) {
	key := sha256.Sum256([]byte("avplatform/secrets/v1\x00" + secretKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	return &Box{aead}, err
}

// Seal returns version || nonce || ciphertext. label binds the value to its
// column and owner so a sealed value cannot be moved elsewhere.
func (b *Box) Seal(label string, plain []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte{version}, nonce...)
	return b.aead.Seal(out, nonce, plain, []byte(label)), nil
}

func (b *Box) Open(label string, sealed []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < 1+n || sealed[0] != version {
		return nil, errors.New("secrets: unknown sealed format")
	}
	return b.aead.Open(nil, sealed[1:1+n], sealed[1+n:], []byte(label))
}
