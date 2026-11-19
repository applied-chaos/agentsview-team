// Package config reads the Platform's settings from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Listen         string
	BaseURL        string // public URL, no trailing slash
	DatabaseURL    string
	SecretKey      string
	Production     bool
	WebDir         string
	DataDir        string
	AgentsviewBin  string
	OIDC           OIDC
	AllowedDomains []string
	RequiredGroup  string
}

type OIDC struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	Scopes       []string
	GroupsClaim  string
	ProviderName string
}

// Load parses and validates the environment.
func Load(getenv func(string) string) (Config, error) {
	get := func(k, d string) string {
		if v := getenv(k); v != "" {
			return v
		}
		return d
	}
	c := Config{
		Listen:        get("PLATFORM_LISTEN", ":8080"),
		BaseURL:       strings.TrimRight(getenv("PLATFORM_BASE_URL"), "/"),
		DatabaseURL:   getenv("PLATFORM_DATABASE_URL"),
		SecretKey:     getenv("PLATFORM_SECRET_KEY"),
		Production:    getenv("PLATFORM_ENV") == "production",
		WebDir:        get("PLATFORM_WEB_DIR", "web/dist"),
		DataDir:       get("PLATFORM_DATA_DIR", "/data"),
		AgentsviewBin: get("PLATFORM_AGENTSVIEW_BIN", "agentsview"),
		OIDC: OIDC{
			Issuer:       getenv("OIDC_ISSUER"),
			ClientID:     getenv("OIDC_CLIENT_ID"),
			ClientSecret: getenv("OIDC_CLIENT_SECRET"),
			Scopes:       strings.Fields(get("OIDC_SCOPES", "openid email profile")),
			GroupsClaim:  get("OIDC_GROUPS_CLAIM", "groups"),
			ProviderName: get("OIDC_PROVIDER_NAME", "SSO"),
		},
		RequiredGroup: getenv("PLATFORM_REQUIRED_GROUP"),
	}
	for _, d := range strings.Split(getenv("PLATFORM_ALLOWED_EMAIL_DOMAINS"), ",") {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			c.AllowedDomains = append(c.AllowedDomains, d)
		}
	}
	var errs []error
	for _, r := range []struct{ k, v string }{
		{"PLATFORM_BASE_URL", c.BaseURL}, {"PLATFORM_DATABASE_URL", c.DatabaseURL}, {"PLATFORM_SECRET_KEY", c.SecretKey},
		{"OIDC_ISSUER", c.OIDC.Issuer}, {"OIDC_CLIENT_ID", c.OIDC.ClientID}, {"OIDC_CLIENT_SECRET", c.OIDC.ClientSecret},
	} {
		if r.v == "" {
			errs = append(errs, fmt.Errorf("%s is required", r.k))
		}
	}
	return c, errors.Join(errs...)
}

// FromEnv loads from the process environment.
func FromEnv() (Config, error) { return Load(os.Getenv) }
