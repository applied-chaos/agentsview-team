package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/applied-chaos/agentsview-team/internal/tokens"
)

const (
	AccessTTL  = 15 * time.Minute
	RefreshTTL = 7 * 24 * time.Hour
	flowTTL    = 10 * time.Minute
)

// AccessClaims is the 15-minute access token. Subject is always the signed-in Admin or User;
// Imp is the Impersonation target, when an Admin is acting as another User.
type AccessClaims struct {
	Admin bool   `json:"adm"`
	Imp   string `json:"imp,omitempty"`
	jwt.RegisteredClaims
}

// Effective is the User the caller acts as.
func (c AccessClaims) Effective() string {
	if c.Imp != "" {
		return c.Imp
	}
	return c.Subject
}

type flowClaims struct {
	State    string `json:"state"`
	Nonce    string `json:"nonce"`
	Verifier string `json:"verifier"`
	jwt.RegisteredClaims
}

type deniedClaims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

type signer struct {
	key []byte
	now func() time.Time
}

func newSigner(secret string, now func() time.Time) signer {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte("platform jwt v1"))
	return signer{key: m.Sum(nil), now: now}
}

func (s signer) sign(c jwt.Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(s.key)
}

func (s signer) parse(raw string, into jwt.Claims) error {
	_, err := jwt.ParseWithClaims(raw, into, func(*jwt.Token) (any, error) { return s.key, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithTimeFunc(s.now), jwt.WithExpirationRequired())
	return err
}

func (s signer) access(userID string, admin bool, imp string) (string, error) {
	now := s.now()
	return s.sign(AccessClaims{Admin: admin, Imp: imp, RegisteredClaims: jwt.RegisteredClaims{
		Subject: userID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(AccessTTL)),
	}})
}

func (s signer) verifyAccess(raw string) (AccessClaims, error) {
	var c AccessClaims
	if err := s.parse(raw, &c); err != nil {
		return c, err
	}
	if c.Subject == "" {
		return c, errors.New("no subject")
	}
	return c, nil
}

func randomToken() (string, error) { return tokens.Random(32) }

func hashToken(t string) []byte { return tokens.Hash(t) }
