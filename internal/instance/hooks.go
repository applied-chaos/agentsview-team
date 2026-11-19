//go:build !testclock

package instance

import "net/http"

func beforeStart(string) error { return nil }

func (*Manager) testRoutes(*http.ServeMux) {}
