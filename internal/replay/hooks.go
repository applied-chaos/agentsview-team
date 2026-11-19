//go:build !testclock

package replay

import "net/http"

type testHooks struct{}

func (testHooks) crashAfterCommit() error { return nil }

func (testHooks) isCrash(error) bool { return false }

func (*Service) TestRoutes(*http.ServeMux) {}
