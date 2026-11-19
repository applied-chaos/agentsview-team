package auth

import (
	"slices"
	"strings"
)

// Rule is the sign-in rule. Every configured criterion must hold.
type Rule struct {
	Domains []string // lower-case, exact match on the email's domain
	Group   string
}

func (r Rule) Permits(email string, groups []string) bool {
	if len(r.Domains) > 0 {
		at := strings.LastIndex(email, "@")
		if at < 0 || !slices.Contains(r.Domains, strings.ToLower(email[at+1:])) {
			return false
		}
	}
	return r.Group == "" || slices.Contains(groups, r.Group)
}
