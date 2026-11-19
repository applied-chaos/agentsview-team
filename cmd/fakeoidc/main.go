// Command fakeoidc is a minimal OIDC issuer for tests and local development.
// The signed-in identity comes from the fake_identity cookie on the issuer host,
// formatted "email|name|group,group", so concurrent clients never share state.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type grant struct {
	email, name, nonce, challenge, redirect string
	groups                                  []string
	verified, fault                         string
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:9000/healthz", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			os.Exit(1)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			os.Exit(1)
		}
		return
	}
	issuer := env("FAKEOIDC_ISSUER", "http://localhost:9000")
	clientID := env("FAKEOIDC_CLIENT_ID", "platform")
	clientSecret := env("FAKEOIDC_CLIENT_SECRET", "platform-secret")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal(err)
	}
	var (
		mu     sync.Mutex
		grants = map[string]grant{}
	)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/authorize",
			"token_endpoint":                        issuer + "/token",
			"jwks_uri":                              issuer + "/jwks",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256", "HS256"},
			"code_challenge_methods_supported":      []string{"S256"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		e := big.NewInt(int64(key.E)).Bytes()
		writeJSON(w, map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "fake",
			"n": b64(key.N.Bytes()), "e": b64(e),
		}}})
	})
	mux.HandleFunc("GET /authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("client_id") != clientID {
			http.Error(w, "bad client", http.StatusBadRequest)
			return
		}
		c, err := r.Cookie("fake_identity")
		if err != nil {
			http.Error(w, "no fake_identity cookie", http.StatusBadRequest)
			return
		}
		raw, _ := url.QueryUnescape(c.Value)
		parts := strings.Split(raw, "|")
		if len(parts) < 3 || len(parts) > 5 {
			http.Error(w, "bad fake_identity cookie", http.StatusBadRequest)
			return
		}
		g := grant{email: parts[0], name: parts[1], nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri")}
		if len(parts) > 3 {
			g.verified = parts[3]
		}
		if len(parts) > 4 {
			g.fault = parts[4]
		}
		if parts[2] != "" {
			g.groups = strings.Split(parts[2], ",")
		}
		code := randomHex()
		mu.Lock()
		grants[code] = g
		mu.Unlock()
		u, err := url.Parse(g.redirect)
		if err != nil {
			http.Error(w, "bad redirect", http.StatusBadRequest)
			return
		}
		rq := u.Query()
		rq.Set("code", code)
		rq.Set("state", q.Get("state"))
		u.RawQuery = rq.Encode()
		http.Redirect(w, r, u.String(), http.StatusFound)
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		id, secret, ok := r.BasicAuth()
		if !ok {
			id, secret = r.FormValue("client_id"), r.FormValue("client_secret")
		}
		if id != clientID || secret != clientSecret {
			http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
			return
		}
		mu.Lock()
		g, found := grants[r.FormValue("code")]
		delete(grants, r.FormValue("code"))
		mu.Unlock()
		sum := sha256.Sum256([]byte(r.FormValue("code_verifier")))
		if !found || g.challenge != b64(sum[:]) || g.redirect != r.FormValue("redirect_uri") {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		now := time.Now()
		claims := jwt.MapClaims{
			"iss": issuer, "aud": clientID, "sub": "sub-" + g.email,
			"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "nonce": g.nonce,
			"email": g.email, "name": g.name, "groups": g.groups,
		}
		switch g.verified {
		case "", "true":
			claims["email_verified"] = true
		case "false":
			claims["email_verified"] = false
		}
		if g.fault == "nonce" {
			claims["nonce"] = "wrong"
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = "fake"
		if g.fault == "rs512" { // an algorithm discovery does not list
			tok = jwt.NewWithClaims(jwt.SigningMethodRS512, claims)
			tok.Header["kid"] = "fake"
		}
		var signKey any = key
		if g.fault == "hs256" || g.fault == "hs256bad" {
			tok = jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
			signKey = []byte(clientSecret)
			if g.fault == "hs256bad" {
				signKey = []byte("not-the-secret")
			}
		}
		signed, err := tok.SignedString(signKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"access_token": randomHex(), "token_type": "Bearer", "expires_in": 300, "id_token": signed})
	})
	log.Fatal(http.ListenAndServe(env("FAKEOIDC_LISTEN", ":9000"), mux))
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func randomHex() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
