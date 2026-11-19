package replay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	chunkBytes     = 4 << 20
	tokenTTL       = 13 * time.Minute // upstream issues 15 minute tokens
	maxOffsetFixes = 8
)

type conflict struct {
	Code           string `json:"code"`
	UploadOffset   int64  `json:"upload_offset"`
	CurrentReceipt string `json:"current_receipt"`
}

func (c *conflict) Error() string { return "admin instance conflict: " + c.Code }

// client speaks upstream's raw-sync protocol to the Admin Instance on loopback, as one mirrored device.
type client struct {
	target     string // Admin Instance origin the client was built for
	base       string // http://127.0.0.1:port/i/<id>
	deviceID   string
	credential string

	mu    sync.Mutex
	token string
	until time.Time
}

func newClient(target *url.URL, adminID, deviceID, credential string) *client {
	return &client{target: target.String(), base: target.String() + "/i/" + adminID + "/api/v1/raw-sync", deviceID: deviceID, credential: credential}
}

type reply struct {
	status int
	header http.Header
	data   []byte
}

func (c *client) do(ctx context.Context, method, path string, hdr map[string]string, body []byte) (reply, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return reply{}, err
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return reply{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	return reply{resp.StatusCode, resp.Header, data}, err
}

func (c *client) accessToken(ctx context.Context, fresh bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !fresh && c.token != "" && time.Now().Before(c.until) {
		return c.token, nil
	}
	r, err := c.do(ctx, http.MethodPost, "/tokens", map[string]string{
		"Authorization": "Bearer " + c.credential, "X-AgentsView-Device-ID": c.deviceID, "Content-Type": "application/json",
	}, []byte(`{"scopes":["negotiate","upload","commit"]}`))
	if err != nil {
		return "", err
	}
	var out struct {
		Token string `json:"token"`
	}
	if r.status != http.StatusOK || json.Unmarshal(r.data, &out) != nil || out.Token == "" {
		return "", fmt.Errorf("admin instance token: %d %s", r.status, strings.TrimSpace(string(r.data)))
	}
	c.token, c.until = out.Token, time.Now().Add(tokenTTL)
	return c.token, nil
}

// call sends an authorised request, refreshing the token once on 401. Any status is returned.
func (c *client) call(ctx context.Context, method, path string, hdr map[string]string, body []byte) (reply, error) {
	for attempt := 0; ; attempt++ {
		tok, err := c.accessToken(ctx, attempt > 0)
		if err != nil {
			return reply{}, err
		}
		h := map[string]string{"Authorization": "Bearer " + tok}
		for k, v := range hdr {
			h[k] = v
		}
		r, err := c.do(ctx, method, path, h, body)
		if err != nil || r.status != http.StatusUnauthorized || attempt > 0 {
			return r, err
		}
	}
}

func asConflict(r reply) error {
	if r.status == http.StatusConflict {
		var c conflict
		if json.Unmarshal(r.data, &c) == nil {
			return &c
		}
	}
	return fmt.Errorf("admin instance %d: %s", r.status, strings.TrimSpace(string(r.data)))
}

func (c *client) Missing(ctx context.Context, provider string, refs []ObjectRef) ([]ObjectRef, error) {
	body, _ := json.Marshal(map[string]any{"provider": provider, "objects": refs})
	r, err := c.call(ctx, http.MethodPost, "/objects/missing", map[string]string{"Content-Type": "application/json"}, body)
	if err != nil {
		return nil, err
	}
	if r.status != http.StatusOK {
		return nil, asConflict(r)
	}
	var out struct {
		Missing []ObjectRef `json:"missing"`
	}
	err = json.Unmarshal(r.data, &out)
	return out.Missing, err
}

type uploadState struct {
	UploadID string `json:"upload_id"`
	Offset   int64  `json:"offset"`
	Complete bool   `json:"complete"`
}

// offsetOf prefers the headers over the body, as upstream's client does.
func offsetOf(r reply) uploadState {
	var st uploadState
	_ = json.Unmarshal(r.data, &st)
	if v, err := strconv.ParseInt(r.header.Get("Upload-Offset"), 10, 64); err == nil {
		st.Offset = v
	}
	if h := r.header.Get("Upload-Complete"); h != "" {
		st.Complete = h == "true"
	}
	return st
}

// Upload sends one object from r (resumable: the server's offset wins).
func (c *client) Upload(ctx context.Context, provider string, o ObjectRef, src io.ReaderAt) error {
	body, _ := json.Marshal(map[string]any{"provider": provider, "object": o})
	r, err := c.call(ctx, http.MethodPost, "/uploads", map[string]string{"Content-Type": "application/json"}, body)
	if err != nil {
		return err
	}
	if r.status/100 != 2 {
		return asConflict(r)
	}
	st := offsetOf(r)
	if st.Complete && st.UploadID == "" {
		return nil
	}
	for fixes := 0; !st.Complete; {
		n := min(chunkBytes, o.Length-st.Offset)
		chunk := make([]byte, n)
		if got, err := src.ReadAt(chunk, st.Offset); got != len(chunk) && err != nil {
			return err
		}
		r, err := c.call(ctx, http.MethodPatch, "/uploads/"+st.UploadID, map[string]string{
			"Upload-Offset": strconv.FormatInt(st.Offset, 10), "Content-Type": "application/octet-stream",
		}, chunk)
		if err != nil {
			return err
		}
		if r.status == http.StatusOK {
			st = offsetOf(r)
			continue
		}
		var cf *conflict
		if err := asConflict(r); !errors.As(err, &cf) || fixes >= maxOffsetFixes ||
			(cf.Code != "upload_offset_conflict" && cf.Code != "checksum_mismatch") {
			return err
		}
		fixes++
		st.Offset = cf.UploadOffset
	}
	return nil
}

func (c *client) Commit(ctx context.Context, manifest []byte) (receipt string, err error) {
	r, err := c.call(ctx, http.MethodPost, "/manifests", map[string]string{"Content-Type": "application/json"}, manifest)
	if err != nil {
		return "", err
	}
	if r.status != http.StatusOK {
		return "", asConflict(r)
	}
	var out struct {
		Receipt string `json:"receipt"`
	}
	if err := json.Unmarshal(r.data, &out); err != nil || out.Receipt == "" {
		return "", fmt.Errorf("admin instance commit reply: %s", strings.TrimSpace(string(r.data)))
	}
	return out.Receipt, nil
}
