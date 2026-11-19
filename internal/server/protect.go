package server

import (
	"net/http"
	"strings"
)

// spaCSP is for the Platform's own pages; Instance content under /i/ is agentsview's and is not constrained.
const spaCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https://www.gravatar.com; " +
	"font-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// protect refuses cross-site state-changing requests (Sec-Fetch-Site, else Origin versus Host;
// requests with neither header, such as install scripts, pass) and stops the Platform being framed.
func protect(next http.Handler) http.Handler {
	checked := http.NewCrossOriginProtection().Handler(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		if !strings.HasPrefix(r.URL.Path, "/i/") {
			h.Set("Content-Security-Policy", spaCSP)
		}
		checked.ServeHTTP(w, r)
	})
}
