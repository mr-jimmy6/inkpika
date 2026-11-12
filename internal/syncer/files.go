package syncer

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/download"
)

type metadata struct {
	Size       int64  `json:"size"`
	ETag       string `json:"etag"`
	Modified   int64  `json:"modified"`
	Version    string `json:"version"`
	MD5        string `json:"md5"`
	Attributes string `json:"attributes"`
}

func digestMD5(value string) (string, error) {
	value = strings.Trim(strings.TrimSpace(value), `"`)
	if b, err := hex.DecodeString(value); err == nil && len(b) == 16 {
		return hex.EncodeToString(b), nil
	}
	if b, err := base64.StdEncoding.DecodeString(value); err == nil && len(b) == 16 {
		return hex.EncodeToString(b), nil
	}
	return "", errors.New("Invalid remote MD5 metadata")
}
func metadataOf(r *http.Response) (metadata, error) {
	m := metadata{Size: r.ContentLength, ETag: r.Header.Get("ETag")}
	if r.StatusCode != http.StatusOK || m.Size < 0 {
		return m, errors.New("Remote response lacks complete file metadata or size")
	}
	if t, err := http.ParseTime(r.Header.Get("Last-Modified")); err == nil {
		m.Modified = t.Unix()
	}
	attrs := map[string]string{}
	if generation := r.Header.Get("X-Goog-Metageneration"); generation != "" {
		attrs["x-goog-metageneration"] = generation
	}
	for name, values := range r.Header {
		k := strings.ToLower(name)
		if strings.HasPrefix(k, "x-oss-meta-") || strings.HasPrefix(k, "x-qs-meta-") || strings.HasPrefix(k, "x-amz-meta-") || strings.HasPrefix(k, "x-ms-meta-") || strings.HasPrefix(k, "x-goog-meta-") || strings.HasPrefix(k, "x-cos-meta-") || strings.HasPrefix(k, "x-tos-meta-") {
			attrs[k] = strings.Join(values, ",")
		}
	}
	b, _ := json.Marshal(attrs)
	m.Attributes = string(b)
	m.Version = r.Header.Get("X-Oss-Version-Id")
	if m.Version == "" {
		m.Version = r.Header.Get("X-QS-Version-Id")
	}
	if m.Version == "" {
		m.Version = r.Header.Get("X-Amz-Version-Id")
	}
	if m.Version == "" {
		for _, key := range []string{"X-Ms-Version-Id", "X-Goog-Generation", "X-Cos-Version-Id", "X-Tos-Version-Id"} {
			if value := r.Header.Get(key); value != "" {
				m.Version = value
				break
			}
		}
	}
	if m.Version == "null" {
		m.Version = ""
	}
	for _, key := range []string{"Content-MD5", "X-Oss-Meta-Md5", "X-QS-Meta-Md5", "X-Amz-Meta-Md5", "X-Ms-Meta-Md5", "X-Goog-Meta-Md5", "X-Cos-Meta-Md5", "X-Tos-Meta-Md5"} {
		if v := r.Header.Get(key); v != "" {
			d, err := digestMD5(v)
			if err != nil {
				return m, err
			}
			if m.MD5 != "" && m.MD5 != d {
				return m, errors.New("Conflicting remote MD5 metadata")
			}
			m.MD5 = d
		}
	}
	return m, nil
}
func (m metadata) stable() bool {
	return m.ETag != "" || m.Version != "" || m.Modified > 0 || m.MD5 != "" || m.Attributes != "{}"
}
func (e *Engine) head(ctx context.Context, key string) (metadata, error) {
	r, err := e.store.Head(ctx, key)
	if err != nil {
		return metadata{}, err
	}
	if r.Body != nil {
		r.Body.Close()
	}
	return metadataOf(r)
}
func hashes(ctx context.Context, filename string) (string, string, error) {
	st, err := os.Lstat(filename)
	if err != nil {
		return "", "", err
	}
	if !st.Mode().IsRegular() {
		return "", "", errors.New("Target is not a regular file")
	}
	f, err := os.Open(filename)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	sha := sha256.New()
	md := md5.New()
	buf := make([]byte, 32*1024)
	for {
		if err = ctx.Err(); err != nil {
			return "", "", err
		}
		n, readErr := f.Read(buf)
		sha.Write(buf[:n])
		md.Write(buf[:n])
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", "", readErr
		}
	}
	return hex.EncodeToString(sha.Sum(nil)), hex.EncodeToString(md.Sum(nil)), nil
}
func (e *Engine) record(key string, m metadata, sha string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.state.Receipts[key] = receipt{Meta: m, SHA256: sha}
	return e.persistLocked()
}
func (e *Engine) current(ctx context.Context, key string, m metadata) (bool, string, error) {
	e.mu.Lock()
	r, ok := e.state.Receipts[key]
	e.mu.Unlock()
	st, err := os.Lstat(e.downloads.Target(key))
	if os.IsNotExist(err) {
		return false, "Not downloaded", nil
	}
	if err != nil || !st.Mode().IsRegular() {
		return false, "Local file unavailable", nil
	}
	if st.Size() != m.Size {
		return false, "File size changed", nil
	}
	if ok && r.Meta != m {
		return false, "Remote version changed", nil
	}
	if !m.stable() {
		return false, "Remote version information missing; verification required", nil
	}
	sha, md, err := hashes(ctx, e.downloads.Target(key))
	if err != nil {
		if ctx.Err() != nil {
			return false, "", ctx.Err()
		}
		return false, "Local verification failed", nil
	}
	if ok && r.SHA256 == sha {
		return true, "", nil
	}
	// Only explicit MD5 is a checksum. An ETag (including a 32-character one)
	// remains an opaque version validator, e.g. multipart/encrypted OSS objects.
	if !ok && m.MD5 != "" && m.MD5 == md {
		return true, "", e.record(key, m, sha)
	}
	if ok {
		return false, "Local checksum mismatch", nil
	}
	return false, "Local file exists; remote verification pending", nil
}
func (e *Engine) fetch(ctx context.Context, key string) (bool, error) {
	m, err := e.head(ctx, key)
	if err != nil {
		return false, err
	}
	current, _, err := e.current(ctx, key, m)
	if err != nil || current {
		return false, err
	}
	if m.Size > e.cfg.MaxDownloadMB*1024*1024 {
		return false, errors.New("File exceeds the configured size limit")
	}
	if free, err := download.FreeBytes(e.cfg.StateDir); err != nil {
		return false, err
	} else if uint64(m.Size)+8*1024*1024 > free {
		return false, errors.New("Insufficient Kindle storage (8 MB reserved)")
	}
	target := e.downloads.Target(key)
	original, statErr := os.Lstat(target)
	if statErr != nil && !os.IsNotExist(statErr) {
		return false, statErr
	}
	if statErr == nil && !original.Mode().IsRegular() {
		return false, errors.New("Target is not a regular file; not replaced")
	}
	e.mu.Lock()
	_, owned := e.state.Receipts[key]
	e.mu.Unlock()
	part, err := os.CreateTemp(filepath.Join(e.cfg.StateDir, "parts"), "sync-*.part")
	if err != nil {
		return false, err
	}
	defer os.Remove(part.Name())
	defer part.Close()
	bodyCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	idle := time.AfterFunc(60*time.Second, cancel)
	defer idle.Stop()
	r, err := e.store.Get(bodyCtx, key, 0, m.ETag)
	if err != nil {
		return false, err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK || (r.ContentLength >= 0 && r.ContentLength != m.Size) {
		return false, errors.New("Unexpected download response size or status")
	}
	if tag := r.Header.Get("ETag"); m.ETag != "" && tag != m.ETag {
		return false, errors.New("Download version changed")
	}
	sha := sha256.New()
	md := md5.New()
	writer := &idleWriter{writer: io.MultiWriter(part, sha, md), idle: idle}
	n, err := io.CopyBuffer(writer, io.LimitReader(r.Body, m.Size+1), make([]byte, 32*1024))
	if err != nil {
		return false, fmt.Errorf("Download interrupted; old file preserved: %w", err)
	}
	if n != m.Size {
		return false, errors.New("File incomplete; old file preserved")
	}
	hash := hex.EncodeToString(sha.Sum(nil))
	mdHash := hex.EncodeToString(md.Sum(nil))
	if m.MD5 != "" && mdHash != m.MD5 {
		return false, errors.New("MD5 verification failed; old file preserved")
	}
	if v := r.Header.Get("Content-MD5"); v != "" {
		d, err := digestMD5(v)
		if err != nil || d != mdHash {
			return false, errors.New("Response MD5 verification failed")
		}
	}
	// Recheck after receiving the body, including version/custom metadata changes
	// that If-Match alone cannot detect. Never publish a mixed-version download.
	after, err := e.head(ctx, key)
	if err != nil {
		return false, err
	}
	if after != m {
		return false, errors.New("Remote file changed during download; try again. Old file preserved")
	}
	if err = ctx.Err(); err != nil {
		return false, err
	}
	if err = part.Sync(); err != nil {
		return false, err
	}
	if err = part.Close(); err != nil {
		return false, err
	}
	now, nowErr := os.Lstat(target)
	if statErr == nil {
		if nowErr != nil || !os.SameFile(original, now) || !now.Mode().IsRegular() {
			return false, errors.New("Local file changed during download; not replaced")
		}
		if !owned {
			local, _, err := hashes(ctx, target)
			if err != nil {
				return false, err
			}
			if local != hash {
				return false, errors.New("An unregistered different local copy exists; move it first. Nothing overwritten")
			}
			return true, e.record(key, m, hash)
		}
	} else if !os.IsNotExist(nowErr) {
		return false, errors.New("Target file appeared during download; not overwritten")
	}
	if err = os.Rename(part.Name(), target); err != nil {
		return false, fmt.Errorf("Save failed; state_dir and download_dir must be on the same filesystem: %w", err)
	}
	return true, e.record(key, m, hash)
}

type idleWriter struct {
	writer io.Writer
	idle   *time.Timer
}

func (w *idleWriter) Write(b []byte) (int, error) {
	n, err := w.writer.Write(b)
	w.idle.Reset(60 * time.Second)
	return n, err
}
