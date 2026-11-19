package config

import "testing"

func TestNoDomainsAndNoGroupIsAllowed(t *testing.T) {
	env := map[string]string{
		"PLATFORM_BASE_URL": "https://p.example", "PLATFORM_DATABASE_URL": "postgres://x", "PLATFORM_SECRET_KEY": "k",
		"OIDC_ISSUER": "https://i.example", "OIDC_CLIENT_ID": "c", "OIDC_CLIENT_SECRET": "s",
	}
	if _, err := Load(func(k string) string { return env[k] }); err != nil {
		t.Fatal(err)
	}
}
