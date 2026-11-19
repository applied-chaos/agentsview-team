package replay

import (
	"context"
	"net/url"
	"testing"

	"github.com/applied-chaos/agentsview-team/internal/store"
)

type movingAdmin struct{ target *url.URL }

func (a *movingAdmin) AdminID() string { return "admin" }
func (a *movingAdmin) Target(string) (*url.URL, string, bool) {
	return a.target, "", true
}
func (*movingAdmin) UpsertDevice(context.Context, string, string, string, string) error { return nil }
func (*movingAdmin) RevokeDevice(context.Context, string, string) error                 { return nil }

// A restarted Admin Instance on a longer port number must yield a fresh client, not a panic.
func TestClientFollowsTheAdminInstanceToALongerAddress(t *testing.T) {
	adm := &movingAdmin{target: &url.URL{Scheme: "http", Host: "127.0.0.1:9999"}}
	s := &Service{admin: adm, clients: map[string]*client{}}
	d := store.ReplayDevice{DeviceID: "dev_1"}
	first, err := s.client(d, "cred")
	if err != nil {
		t.Fatal(err)
	}
	adm.target = &url.URL{Scheme: "http", Host: "127.0.0.1:10000"}
	second, err := s.client(d, "cred")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("client kept pointing at the old address")
	}
}
