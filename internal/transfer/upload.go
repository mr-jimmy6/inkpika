package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mr-jimmy6/inkpika/internal/bookformat"
	"github.com/mr-jimmy6/inkpika/internal/download"
)

func uploadName(raw string, formats []string) (string, error) {
	name, err := url.PathUnescape(raw)
	if err != nil || !utf8.ValidString(name) || len(name) > 1024 || strings.ContainsAny(name, "/\\") || name == "" {
		return "", errors.New("Invalid filename")
	}
	if !bookformat.Allows(formats, name) {
		return "", errors.New("This format is not in the allowed ebook formats: " + strings.Join(formats, "、"))
	}
	ext := strings.ToLower(filepath.Ext(name))
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	stem = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"|?*`, r) {
			return '_'
		}
		return r
	}, stem)
	runes := []rune(strings.Trim(stem, ". "))
	if len(runes) > 65 {
		runes = runes[:65]
	}
	if len(runes) == 0 {
		return "", errors.New("Invalid filename")
	}
	return string(runes) + ext, nil
}

// A single transfer process owns this directory's upload stream. Cloud source
// names/receipts are never used or altered. Existing files (including symlinks)
// are retained; suffixes distinguish collisions within the upload directory.
func (s *Server) receive(w http.ResponseWriter, r *http.Request) {
	formats, err := bookformat.FromFile(s.cfg.FormatsFile)
	if err != nil {
		fail(w, 503, err.Error())
		return
	}
	name, err := uploadName(r.Header.Get("X-File-Name"), formats)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	limit := s.cfg.MaxUploadMB * 1024 * 1024
	if r.ContentLength > limit {
		fail(w, 413, "File exceeds the per-book size limit")
		return
	}
	select {
	case s.upload <- struct{}{}:
		defer func() { <-s.upload }()
	default:
		fail(w, 409, "Kindle is receiving another book; try again shortly")
		return
	}
	free, err := download.FreeBytes(s.cfg.DownloadDir)
	if err != nil || free < 16*1024*1024 || (r.ContentLength > 0 && uint64(r.ContentLength) > free-16*1024*1024) {
		fail(w, 507, "Insufficient free space on Kindle")
		return
	}
	f, err := os.CreateTemp(s.cfg.DownloadDir, ".wireless-upload-*")
	if err != nil {
		fail(w, 500, "Cannot create receiving file; check free space and folder permissions")
		return
	}
	defer os.Remove(f.Name())
	defer f.Close()
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	hash := sha256.New()
	n, err := io.CopyBuffer(io.MultiWriter(f, hash), r.Body, make([]byte, 32*1024))
	if err != nil {
		var oversized *http.MaxBytesError
		if errors.As(err, &oversized) {
			fail(w, 413, "File exceeds the per-book size limit")
		} else {
			fail(w, 400, "Transfer incomplete; upload again")
		}
		return
	}
	if n == 0 || (r.ContentLength >= 0 && n != r.ContentLength) || r.Context().Err() != nil {
		fail(w, 400, "Transfer incomplete or file empty; upload again")
		return
	}
	if err = f.Sync(); err != nil {
		fail(w, 507, "Save failed; check free space on Kindle")
		return
	}
	if err = f.Close(); err != nil {
		fail(w, 500, "Cannot save file")
		return
	}
	stem, ext := strings.TrimSuffix(name, filepath.Ext(name)), filepath.Ext(name)
	for i := 0; i < 10000; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, i+1, ext)
		}
		target := filepath.Join(s.cfg.DownloadDir, candidate)
		if _, err = os.Lstat(target); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			fail(w, 500, "Cannot inspect the target file")
			return
		}
		if err = os.Rename(f.Name(), target); err != nil {
			fail(w, 500, "Cannot save book")
			return
		}
		jsonReply(w, 201, map[string]interface{}{"name": candidate, "size": n, "sha256": hex.EncodeToString(hash.Sum(nil))})
		return
	}
	fail(w, 409, "Too many files with this name; rename and try again")
}
