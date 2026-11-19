package instance

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// Hosted agentsview has no /events stream and no session to watch without an
// upload, so streaming through the proxy cannot be driven from the HTTP seam.
func TestProxyAddsBearerAndStreamsEventsUnbuffered(t *testing.T) {
	release := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer instance-token" || r.Header.Get("X-Forwarded-For") == "" || r.Header.Get("Cookie") != "" {
			http.Error(w, "bad headers", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: events_ready\ndata: {}\n\n"))
		w.(http.Flusher).Flush()
		<-release
	}))
	defer up.Close()
	defer close(release)
	target, _ := url.Parse(up.URL)
	front := httptest.NewServer(reverseProxy(target, "instance-token", false, "u", banner{}, nil))
	defer front.Close()

	req, _ := http.NewRequest("GET", front.URL+"/i/u/api/v1/events", nil)
	req.Header.Set("Authorization", "Bearer client-supplied")
	req.Header.Set("Cookie", "av_access=secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	got := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(resp.Body).ReadString('\n')
		got <- line
	}()
	select {
	case line := <-got:
		if line != "event: events_ready\n" {
			t.Fatalf("first line %q", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("event held back by the proxy")
	}
}

func TestRawSyncRequestsKeepTheirOwnAuthorization(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get("Authorization")))
	}))
	defer up.Close()
	target, _ := url.Parse(up.URL)
	front := httptest.NewServer(reverseProxy(target, "instance-token", true, "u", banner{}, nil))
	defer front.Close()
	req, _ := http.NewRequest("POST", front.URL+"/i/u/api/v1/raw-sync/tokens", nil)
	req.Header.Set("Authorization", "Bearer avdc_host")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if string(got) != "Bearer avdc_host" {
		t.Fatalf("upstream saw %q", got)
	}
}
