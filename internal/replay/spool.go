// Package replay feeds the Admin Instance (ADR 0003): the raw-sync pass-through spools every
// verified object and committed manifest, and a worker replays them as mirrored devices.
package replay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

type ObjectRef struct {
	SHA256 string `json:"sha256"`
	Length int64  `json:"length"`
}

var (
	shaRe      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	uploadIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

func (o ObjectRef) valid() bool { return shaRe.MatchString(o.SHA256) && o.Length >= 0 }

func (o ObjectRef) file() string { return fmt.Sprintf("%s-%d", o.SHA256, o.Length) }

type Spool struct{ dir string }

func NewSpool(dir string) (*Spool, error) {
	for _, d := range []string{"objects", "partial"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o700); err != nil {
			return nil, err
		}
	}
	return &Spool{dir: dir}, nil
}

func (s *Spool) objectPath(o ObjectRef) string { return filepath.Join(s.dir, "objects", o.file()) }
func (s *Spool) partPath(id string) string     { return filepath.Join(s.dir, "partial", id) }

func (s *Spool) Has(o ObjectRef) bool {
	st, err := os.Stat(s.objectPath(o))
	return err == nil && st.Size() == o.Length
}

// Remove deletes an object once the Admin Instance holds it; a missing file is fine.
func (s *Spool) Remove(o ObjectRef) error {
	if err := os.Remove(s.objectPath(o)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Spool) Open(o ObjectRef) (*os.File, error) { return os.Open(s.objectPath(o)) }

func (s *Spool) Begin(uploadID string, o ObjectRef) error {
	if !uploadIDRe.MatchString(uploadID) || !o.valid() {
		return errors.New("invalid upload")
	}
	meta, err := json.Marshal(o)
	if err != nil {
		return err
	}
	return os.WriteFile(s.partPath(uploadID)+".json", meta, 0o600)
}

func (s *Spool) object(uploadID string) (ObjectRef, bool) {
	if !uploadIDRe.MatchString(uploadID) {
		return ObjectRef{}, false
	}
	raw, err := os.ReadFile(s.partPath(uploadID) + ".json")
	var o ObjectRef
	if err != nil || json.Unmarshal(raw, &o) != nil {
		return ObjectRef{}, false
	}
	return o, true
}

// Writer returns a writer that puts bytes at the chunk's offset in the partial file, so a
// re-sent chunk overwrites itself; ok is false for an upload the spool never saw start.
func (s *Spool) Writer(uploadID string, offset int64) (w io.WriteCloser, ok bool, err error) {
	if _, ok := s.object(uploadID); !ok {
		return nil, false, nil
	}
	f, err := os.OpenFile(s.partPath(uploadID), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, true, err
	}
	return &offsetWriter{f: f, off: offset}, true, nil
}

type offsetWriter struct {
	f   *os.File
	off int64
}

func (w *offsetWriter) Write(p []byte) (int, error) {
	n, err := w.f.WriteAt(p, w.off)
	w.off += int64(n)
	return n, err
}

func (w *offsetWriter) Close() error { return w.f.Close() }

// Finish verifies a completed upload's digest and length, as the server does, and stores it by
// content address. A bad partial is dropped so the next attempt starts clean.
func (s *Spool) Finish(uploadID string) error {
	o, ok := s.object(uploadID)
	if !ok {
		return nil
	}
	part := s.partPath(uploadID)
	defer func() { _ = os.Remove(part); _ = os.Remove(part + ".json") }()
	f, err := os.OpenFile(part, os.O_RDONLY|os.O_CREATE, 0o600) // a zero-byte object sends no chunk
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return err
	}
	if n != o.Length || hex.EncodeToString(h.Sum(nil)) != o.SHA256 {
		return fmt.Errorf("spooled object %s does not match its address", o.file())
	}
	return os.Rename(part, s.objectPath(o))
}
