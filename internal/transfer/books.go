package transfer

import (
	"archive/zip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/bookformat"
)

type localBook struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Folder   string `json:"folder"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	path     string
	info     os.FileInfo
}

type exportTicket struct {
	books   []localBook
	expires time.Time
}

func (s *Server) catalogue() ([]localBook, error) {
	formats, err := bookformat.FromFile(s.cfg.FormatsFile)
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(s.cfg.LibraryDir)
	if err != nil {
		return nil, errors.New("Cannot read the Kindle local library folder")
	}
	books := []localBook{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("Some book folders cannot be read; check storage permissions")
		}
		if path == root {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") || strings.HasSuffix(d.Name(), ".sdr") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || strings.HasPrefix(d.Name(), ".") || !bookformat.Allows(formats, d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if len(books) >= 20000 {
			return errors.New("Local library exceeds 20000 books; narrow the library folder")
		}
		rel, _ := filepath.Rel(root, path)
		hash := sha256.Sum256([]byte(rel))
		folder := filepath.ToSlash(filepath.Dir(rel))
		if folder == "." {
			folder = ""
		}
		books = append(books, localBook{hex.EncodeToString(hash[:16]), d.Name(), folder, info.Size(), info.ModTime().Unix(), path, info})
		return nil
	})
	return books, err
}

func (s *Server) listBooks(w http.ResponseWriter, r *http.Request) {
	books, err := s.catalogue()
	if err != nil {
		fail(w, 503, err.Error())
		return
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if len(query) > 1024 {
		fail(w, 400, "Search query is too long")
		return
	}
	filtered := []localBook{}
	for _, b := range books {
		if strings.Contains(strings.ToLower(b.Name), query) {
			filtered = append(filtered, b)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		a, b := strings.ToLower(filtered[i].Name), strings.ToLower(filtered[j].Name)
		if a == b {
			return filtered[i].Folder < filtered[j].Folder
		}
		return a < b
	})
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	if offset > len(filtered) {
		offset = len(filtered)
	}
	end := offset + 50
	if end > len(filtered) {
		end = len(filtered)
	}
	jsonReply(w, 200, map[string]interface{}{"books": filtered[offset:end], "total": len(filtered), "offset": offset, "has_more": end < len(filtered)})
}

func (s *Server) prepareExport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var in struct {
		IDs []string `json:"ids"`
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || d.Decode(new(interface{})) != io.EOF || len(in.IDs) == 0 || len(in.IDs) > 50 {
		fail(w, 400, "Select 1–50 books at a time")
		return
	}
	books, err := s.catalogue()
	if err != nil {
		fail(w, 503, err.Error())
		return
	}
	byID := map[string]localBook{}
	for _, b := range books {
		byID[b.ID] = b
	}
	selected := []localBook{}
	seen := map[string]bool{}
	for _, id := range in.IDs {
		b, ok := byID[id]
		if !ok {
			fail(w, 409, "Selected books were removed or their formats are no longer allowed; refresh the list")
			return
		}
		if !seen[id] {
			selected = append(selected, b)
			seen[id] = true
		}
	}
	b := make([]byte, 24)
	if _, err = rand.Read(b); err != nil {
		fail(w, 500, "Cannot prepare download")
		return
	}
	ticket := hex.EncodeToString(b)
	s.mu.Lock()
	for id, t := range s.exports {
		if time.Now().After(t.expires) {
			delete(s.exports, id)
		}
	}
	if len(s.exports) >= 20 {
		s.mu.Unlock()
		fail(w, 429, "Too many pending downloads; try again shortly")
		return
	}
	s.exports[ticket] = exportTicket{selected, time.Now().Add(2 * time.Minute)}
	s.mu.Unlock()
	jsonReply(w, 200, map[string]interface{}{"url": "/api/download?ticket=" + ticket, "count": len(selected)})
}

// Open first and compare inode/metadata before emitting headers: an item that
// changed after selection, or a swapped symlink, must not export another file.
func openBook(b localBook) (*os.File, error) {
	resolved, err := filepath.EvalSymlinks(b.path)
	if err != nil || resolved != b.path {
		return nil, errors.New("Book path changed")
	}
	f, err := os.OpenFile(b.path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(info, b.info) || info.Size() != b.Size || !info.ModTime().Equal(b.info.ModTime()) {
		f.Close()
		return nil, errors.New("Book changed")
	}
	return f, nil
}

func (s *Server) exportBooks(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	ticket, ok := s.exports[r.URL.Query().Get("ticket")]
	delete(s.exports, r.URL.Query().Get("ticket"))
	s.mu.Unlock()
	if !ok || time.Now().After(ticket.expires) {
		fail(w, 410, "Download link expired; select the books again")
		return
	}
	select {
	case s.upload <- struct{}{}:
		defer func() { <-s.upload }()
	default:
		fail(w, 409, "Kindle is transferring a file; try again shortly")
		return
	}
	formats, err := bookformat.FromFile(s.cfg.FormatsFile)
	if err != nil {
		fail(w, 503, err.Error())
		return
	}
	files := []*os.File{}
	defer func() {
		for _, f := range files {
			f.Close()
		}
	}()
	for _, b := range ticket.books {
		if !bookformat.Allows(formats, b.Name) {
			fail(w, 409, "Allowed formats changed; refresh the list")
			return
		}
		f, err := openBook(b)
		if err != nil {
			fail(w, 409, "Selected books were removed or changed; refresh the list")
			return
		}
		files = append(files, f)
	}
	if len(files) == 1 {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": ticket.books[0].Name}))
		http.ServeContent(w, r, ticket.books[0].Name, ticket.books[0].info.ModTime(), files[0])
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="Kindle-books.zip"`)
	z := zip.NewWriter(w)
	for i, b := range ticket.books {
		if r.Context().Err() != nil {
			return
		}
		header := &zip.FileHeader{Name: filepath.ToSlash(filepath.Join(b.Folder, b.Name)), Method: zip.Store}
		header.SetModTime(b.info.ModTime())
		header.SetMode(0600)
		out, err := z.CreateHeader(header)
		if err != nil {
			return
		}
		if n, err := io.CopyN(out, files[i], b.Size); err != nil || n != b.Size {
			panic(http.ErrAbortHandler)
		}
	}
	if err := z.Close(); err != nil {
		panic(http.ErrAbortHandler)
	}
}
