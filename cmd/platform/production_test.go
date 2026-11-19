//go:build testclock

package main

import (
	"strings"
	"testing"
)

func TestTestClockBuildRefusesToStartInProduction(t *testing.T) {
	for k, v := range map[string]string{
		"PLATFORM_ENV": "production", "PLATFORM_BASE_URL": "https://x.example", "PLATFORM_DATABASE_URL": "postgres://unused",
		"PLATFORM_SECRET_KEY": "k", "OIDC_ISSUER": "https://idp.example", "OIDC_CLIENT_ID": "c", "OIDC_CLIENT_SECRET": "s",
		"PLATFORM_ALLOWED_EMAIL_DOMAINS": "x.example",
	} {
		t.Setenv(k, v)
	}
	if err := run(); err == nil || !strings.Contains(err.Error(), "refused in production") {
		t.Fatalf("got %v", err)
	}
}
