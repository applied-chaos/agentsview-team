package replay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/applied-chaos/agentsview-team/internal/clock"
	"github.com/applied-chaos/agentsview-team/internal/instance"
	"github.com/applied-chaos/agentsview-team/internal/store"
)

const maxManifest = 1 << 20

type Devices interface {
	DeviceForToken(ctx context.Context, userID, token string) (string, error)
}

// Tee implements instance.Tee: it copies verified objects and committed manifests off the
// raw-sync pass-through and never alters what the Host and the Instance say to each other.
type Tee struct {
	spool *Spool
	st    *store.Store
	dev   Devices
	clk   *clock.Clock
	nudge func()
}

type responder func(*http.Response) error

func (f responder) Respond(resp *http.Response) error { return f(resp) }

func (t *Tee) Intercept(userID string, r *http.Request) instance.Responder {
	_, tail, ok := strings.Cut(r.URL.Path, "/api/v1/raw-sync/")
	if !ok {
		return nil
	}
	switch {
	case r.Method == http.MethodPost && tail == "uploads":
		return responder(t.afterStart)
	case r.Method == http.MethodPatch && strings.HasPrefix(tail, "uploads/"):
		return t.chunk(strings.TrimPrefix(tail, "uploads/"), r)
	case r.Method == http.MethodPost && tail == "manifests":
		return t.manifest(userID, r)
	}
	return nil
}

func peek(resp *http.Response) ([]byte, error) {
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(data))
	return data, err
}

func (t *Tee) afterStart(resp *http.Response) error {
	if resp.StatusCode/100 != 2 {
		return nil
	}
	data, err := peek(resp)
	if err != nil {
		return nil
	}
	var out struct {
		UploadID string    `json:"upload_id"`
		Object   ObjectRef `json:"object"`
	}
	if json.Unmarshal(data, &out) != nil || out.UploadID == "" {
		return nil
	}
	if err := t.spool.Begin(out.UploadID, out.Object); err != nil {
		slog.Warn("replay: spool upload start", "err", err)
	}
	return nil
}

type teeBody struct {
	io.Reader
	io.Closer
	w io.Closer
}

func (b teeBody) Close() error { _ = b.w.Close(); return b.Closer.Close() }

// chunk writes the bytes to the spool as the Instance receives them, so the Platform holds
// the object whenever the Instance does; the digest is checked when the upload completes.
func (t *Tee) chunk(uploadID string, r *http.Request) instance.Responder {
	off, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil || off < 0 {
		return nil
	}
	w, ok, err := t.spool.Writer(uploadID, off)
	if err != nil {
		slog.Warn("replay: spool chunk", "err", err)
		return nil
	}
	if ok {
		r.Body = teeBody{Reader: io.TeeReader(r.Body, w), Closer: r.Body, w: w}
	}
	return responder(func(resp *http.Response) error {
		if ok && resp.StatusCode == http.StatusOK && resp.Header.Get("Upload-Complete") == "true" {
			if err := t.spool.Finish(uploadID); err != nil {
				slog.Warn("replay: spool object", "upload", uploadID, "err", err)
			}
		}
		return nil
	})
}

// manifest spools the Host's manifest once the Instance has committed it. A failed spool
// fails the response, so the Host retries the commit (idempotent upstream) and we spool again.
func (t *Tee) manifest(userID string, r *http.Request) instance.Responder {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxManifest+1))
	if err != nil {
		return nil
	}
	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), r.Body))
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return responder(func(resp *http.Response) error {
		if resp.StatusCode != http.StatusOK || len(body) > maxManifest {
			return nil
		}
		data, err := peek(resp)
		if err != nil {
			return nil
		}
		var out struct {
			ManifestID string `json:"manifest_id"`
		}
		if json.Unmarshal(data, &out) != nil || out.ManifestID == "" {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Second)
		defer cancel()
		device, err := t.dev.DeviceForToken(ctx, userID, bearer)
		if err != nil {
			return fmt.Errorf("replay: resolve device: %w", err)
		}
		if device == "" {
			return errors.New("replay: manifest from an unknown device")
		}
		if err := t.st.SpoolManifest(ctx, userID, device, out.ManifestID, body, t.clk.Now()); err != nil {
			return fmt.Errorf("replay: spool manifest: %w", err)
		}
		t.nudge()
		return nil
	})
}
