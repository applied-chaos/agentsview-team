package server

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/applied-chaos/agentsview-team/internal/httpx"
)

// fallbackReason picks the error page for a plain-text failure that named no reason.
func fallbackReason(status int) string {
	switch {
	case status == http.StatusUnauthorized:
		return "unauthorised"
	case status == http.StatusForbidden:
		return "forbidden"
	case status == http.StatusNotFound:
		return "not-found"
	case status == http.StatusBadGateway, status == http.StatusServiceUnavailable, status == http.StatusGatewayTimeout:
		return "unavailable"
	case status >= 500:
		return "server-error"
	}
	return "bad-request"
}

// errorPages turns plain-text failures of page navigations into the SPA's error page, with the
// original status. JSON API calls and non-browser clients keep their plain responses. The SPA
// reads the reason from a meta tag injected into index.html.
func errorPages(webDir string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.Header.Get("Accept"), "text/html") || strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(&errorWriter{ResponseWriter: w, webDir: webDir}, r)
	})
}

type errorWriter struct {
	http.ResponseWriter
	webDir  string
	swapped bool
}

func (e *errorWriter) Unwrap() http.ResponseWriter { return e.ResponseWriter }

func (e *errorWriter) WriteHeader(status int) {
	h := e.Header()
	reason := h.Get(httpx.ReasonHeader)
	if status < 400 || (reason == "" && !strings.HasPrefix(h.Get("Content-Type"), "text/plain")) {
		e.ResponseWriter.WriteHeader(status)
		return
	}
	if reason == "" {
		reason = fallbackReason(status)
	}
	index, err := os.ReadFile(filepath.Join(e.webDir, "index.html"))
	if err != nil {
		e.ResponseWriter.WriteHeader(status)
		return
	}
	meta := `<meta name="av-error" content="` + reason + `">`
	page := bytes.Replace(index, []byte("</head>"), []byte(meta+"</head>"), 1)
	h.Del(httpx.ReasonHeader)
	h.Del("Content-Length")
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	e.swapped = true
	e.ResponseWriter.WriteHeader(status)
	_, _ = e.ResponseWriter.Write(page)
}

func (e *errorWriter) Write(b []byte) (int, error) {
	if e.swapped {
		return len(b), nil
	}
	return e.ResponseWriter.Write(b)
}
