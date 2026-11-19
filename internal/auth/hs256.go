package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const idTokenSkew = 5 * time.Minute

// verifyHS256 checks an ID token signed with the client secret, as Authentik does when no signing
// key is set (go-oidc only handles asymmetric keys). Plain HMAC, as go-jose refuses secrets under
// 32 bytes. Applies go-oidc's checks: issuer, audience, expiry; the caller checks the nonce.
func verifyHS256(raw, secret, issuer, clientID string, now time.Time) (map[string]any, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, errors.New("hs256 token is not a compact JWS")
	}
	got, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("hs256 signature encoding: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(got, mac.Sum(nil)) {
		return nil, errors.New("hs256 signature does not match the client secret")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("hs256 payload encoding: %w", err)
	}
	var c struct {
		Iss string `json:"iss"`
		Aud any    `json:"aud"`
		Exp *int64 `json:"exp"`
		Iat *int64 `json:"iat"`
	}
	claims := map[string]any{}
	if err := errors.Join(json.Unmarshal(payload, &c), json.Unmarshal(payload, &claims)); err != nil {
		return nil, fmt.Errorf("hs256 claims: %w", err)
	}
	if c.Iss != issuer {
		return nil, fmt.Errorf("issuer %q does not match %q", c.Iss, issuer)
	}
	if !slices.Contains(stringList(c.Aud), clientID) {
		return nil, errors.New("audience does not include the client id")
	}
	if c.Exp == nil || now.After(time.Unix(*c.Exp, 0).Add(idTokenSkew)) {
		return nil, errors.New("token expired or without expiry")
	}
	if c.Iat != nil && time.Unix(*c.Iat, 0).After(now.Add(idTokenSkew)) {
		return nil, errors.New("token issued in the future")
	}
	return claims, nil
}
