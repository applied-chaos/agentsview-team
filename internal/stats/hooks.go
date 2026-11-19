//go:build !testclock

package stats

import "net/http"

func (*Service) TestRoutes(*http.ServeMux) {}
