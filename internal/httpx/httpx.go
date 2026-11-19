// Package httpx is the Platform's shared JSON response and request-body handling.
package httpx

import (
	"encoding/json"
	"net/http"
)

// JSON writes v. Responses are never cacheable: they can carry Credentials and tokens.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ReasonHeader names the error page a browser should see for a failed response.
const ReasonHeader = "X-Error-Reason"

// Page is http.Error for browser-facing failures: the plain text stays for non-browsers,
// browsers get the styled error page for reason (see server.errorPages).
func Page(w http.ResponseWriter, status int, reason, msg string) {
	w.Header().Set(ReasonHeader, reason)
	http.Error(w, msg, status)
}

func Fail(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

// Decode reads a JSON body of at most limit bytes into v.
func Decode(w http.ResponseWriter, r *http.Request, limit int64, v any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(v)
}
