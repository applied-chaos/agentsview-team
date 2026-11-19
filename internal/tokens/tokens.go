// Package tokens is the Platform's one place for secrets: hashing, randomness and raw-sync device minting.
package tokens

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// Hash is how tokens are stored: only the digest, never the secret.
func Hash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func Random(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewDevice mints a raw-sync device id and its Credential.
func NewDevice() (deviceID, credential string, err error) {
	id, err := Random(16)
	if err != nil {
		return "", "", err
	}
	secret, err := Random(32)
	if err != nil {
		return "", "", err
	}
	return "dev_" + id, "avdc_" + secret, nil
}
