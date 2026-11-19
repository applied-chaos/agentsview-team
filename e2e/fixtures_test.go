//go:build e2e

package e2e

import (
	"strings"
	"sync"
	"testing"
)

func (b *browser) with(t *testing.T) *browser {
	c := *b
	c.t = t
	return &c
}

func signInAs(t *testing.T, email string) *browser {
	t.Helper()
	b := newBrowser(t)
	b.setIdentity(email, "Person", staff)
	b.signIn()
	b.mustBeSignedIn(email)
	return b
}

// fx is the signed-in User, with a running Instance, that a suite's subtests share.
// Each subtest picks its own names and ids so none depends on another's leftovers.
type fx struct {
	b           *browser
	email, base string
}

func newFx(t *testing.T, prefix string) *fx {
	t.Helper()
	b, email, base := running(t, prefix)
	return &fx{b: b, email: email, base: base}
}

func (f *fx) with(t *testing.T) *fx {
	c := *f
	c.b = f.b.with(t)
	return &c
}

func (f *fx) id() string { return idOf(f.base) }

func (f *fx) run(t *testing.T, name string, fn func(*testing.T, *fx)) {
	t.Helper()
	t.Run(name, func(t *testing.T) { fn(t, f.with(t)) })
}

// sharedUser is one User for the whole run (an Admin, or a plain User who owns nothing), signed in
// afresh for each caller: a new session costs nothing, a new User costs an Instance.
type sharedUser struct {
	prefix string
	admin  bool
	once   sync.Once
	email  string
}

var (
	plainUser = &sharedUser{prefix: "plain"}
	adminUser = &sharedUser{prefix: "admin", admin: true}
)

func (s *sharedUser) browser(t *testing.T) *browser {
	t.Helper()
	s.once.Do(func() { _, s.email = signedIn(t, s.prefix) })
	if s.email == "" {
		t.Fatalf("shared %s User was not created", s.prefix)
	}
	if s.admin {
		sql(t, `update users set is_admin = true where email = $1`, s.email)
	}
	return signInAs(t, s.email)
}

func (s *sharedUser) instance(t *testing.T) (*browser, string) {
	t.Helper()
	b := s.browser(t)
	return b, b.waitInstance("running").URL
}

func (b *browser) hostsNamed(prefix string) []hostView {
	var out []hostView
	for _, h := range b.hosts().Hosts {
		if strings.HasPrefix(h.Name, prefix) {
			out = append(out, h)
		}
	}
	return out
}

func auditSince(t *testing.T, b *browser, query string, before int) []map[string]any {
	t.Helper()
	all := auditEntries(t, b, query)
	if len(all) < before {
		t.Fatalf("audit shrank from %d to %d", before, len(all))
	}
	return all[:len(all)-before]
}
