// Package syncer maintains a read-only, source-scoped mirror of remote sync/ books.
package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/download"
	"github.com/mr-jimmy6/inkpika/internal/statefile"
	"github.com/mr-jimmy6/inkpika/internal/storage"
)

const Prefix = "sync/"

type Item struct {
	Key      string `json:"key"`
	Modified int64  `json:"modified"`
	Size     int64  `json:"size"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
}
type Snapshot struct {
	Phase      string `json:"phase"`
	Running    bool   `json:"running"`
	Fresh      bool   `json:"fresh"`
	Known      bool   `json:"known"`
	Total      int    `json:"total"`
	Downloaded int    `json:"downloaded"`
	Pending    int    `json:"pending"`
	Unfinished []Item `json:"unfinished"`
	Books      []Item `json:"books"`
	Current    string `json:"current,omitempty"`
	Fetched    int    `json:"fetched"`
	Skipped    int    `json:"skipped"`
	Updated    int64  `json:"updated"`
	Error      string `json:"error,omitempty"`
}
type receipt struct {
	Meta   metadata `json:"meta"`
	SHA256 string   `json:"sha256"`
}
type diskState struct {
	Source   string             `json:"source"`
	Receipts map[string]receipt `json:"receipts"`
	Snapshot Snapshot           `json:"snapshot"`
}
type Engine struct {
	mu        sync.Mutex
	cfg       config.Config
	store     storage.Store
	downloads *download.Manager
	menu      string
	state     diskState
	cancel    context.CancelFunc
	done      chan struct{}
	closed    bool
}

func New(c config.Config, store storage.Store, downloads *download.Manager, menu string) (*Engine, error) {
	// A previous power loss may leave our private temporary files behind. The
	// caller holds server.lock; browser .part files use a different name scheme.
	parts, err := filepath.Glob(filepath.Join(c.StateDir, "parts", "sync-*.part"))
	if err != nil {
		return nil, err
	}
	for _, part := range parts {
		if st, statErr := os.Lstat(part); statErr == nil && st.Mode().IsRegular() {
			if err = os.Remove(part); err != nil {
				return nil, err
			}
		}
	}
	e := &Engine{cfg: c, store: store, downloads: downloads, menu: menu,
		state: diskState{Source: c.SourceIdentity(), Receipts: map[string]receipt{}, Snapshot: Snapshot{Phase: "idle"}}}
	b, err := os.ReadFile(filepath.Join(c.StateDir, "sync.json"))
	if err == nil {
		if err = json.Unmarshal(b, &e.state); err != nil {
			return nil, fmt.Errorf("Sync state is corrupt; back up sync.json before fixing it: %w", err)
		}
		if e.state.Source != c.SourceIdentity() {
			return nil, errors.New("Sync state belongs to another storage source; use a separate state_dir")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if e.state.Receipts == nil {
		e.state.Receipts = map[string]receipt{}
	}
	e.state.Snapshot.Running = false
	e.state.Snapshot.Fresh = false
	e.state.Snapshot.Current = ""
	e.state.Snapshot.Phase = "idle"
	for i := range e.state.Snapshot.Books {
		if e.state.Snapshot.Books[i].Status == "downloading" {
			e.state.Snapshot.Books[i].Status = "pending"
		}
	}
	for i := range e.state.Snapshot.Unfinished {
		if e.state.Snapshot.Unfinished[i].Status == "downloading" {
			e.state.Snapshot.Unfinished[i].Status = "pending"
		}
	}
	if err = e.persistLocked(); err != nil {
		return nil, err
	}
	return e, nil
}
func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.state.Snapshot
	s.Unfinished = append([]Item{}, s.Unfinished...)
	if s.Books != nil {
		s.Books = append([]Item{}, s.Books...)
	}
	return s
}
func (e *Engine) Start(fetch bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.startLocked(fetch)
}

// Refresh scans when idle and only republishes progress while a transfer runs.
// The decision and start share the lock, so a refresh cannot race another start.
func (e *Engine) Refresh() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancel != nil {
		return e.persistLocked()
	}
	return e.startLocked(false)
}

func (e *Engine) startLocked(fetch bool) error {
	if e.closed {
		return errors.New("Service stopped")
	}
	if e.cancel != nil {
		return errors.New("A sync task is running; refresh status or stop it before retrying")
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.done = make(chan struct{})
	s := &e.state.Snapshot
	s.Running = true
	s.Fresh = false
	s.Phase = "scanning"
	s.Error = ""
	s.Current = ""
	s.Fetched = 0
	s.Skipped = 0
	if err := e.persistLocked(); err != nil {
		cancel()
		e.cancel = nil
		s.Running = false
		close(e.done)
		return err
	}
	go e.run(ctx, fetch)
	return nil
}
func (e *Engine) Cancel() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancel != nil {
		e.cancel()
	}
}

// Wait joins the current operation without repeatedly copying its catalogue.
func (e *Engine) Wait() {
	e.mu.Lock()
	done := e.done
	e.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (e *Engine) Close() {
	e.mu.Lock()
	e.closed = true
	done := e.done
	if e.cancel != nil {
		e.cancel()
	}
	e.mu.Unlock()
	if done != nil {
		<-done
	}
}
func (e *Engine) persistLocked() error {
	b, err := json.MarshalIndent(e.state, "", "  ")
	if err == nil {
		err = statefile.Write(filepath.Join(e.cfg.StateDir, "sync.json"), b)
	}
	if err == nil && e.menu != "" {
		err = RefreshMenu(e.menu, e.cfg, "")
	}
	return err
}
func (e *Engine) publish(fn func(*Snapshot)) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	fn(&e.state.Snapshot)
	return e.persistLocked()
}
func (e *Engine) run(ctx context.Context, fetch bool) {
	err := e.downloads.WithTransfer(ctx, func() error { return e.scan(ctx, fetch) })
	e.mu.Lock()
	s := &e.state.Snapshot
	s.Running = false
	s.Current = ""
	for i := range s.Books {
		if s.Books[i].Status == "downloading" {
			s.Books[i].Status = "pending"
		}
	}
	for i := range s.Unfinished {
		if s.Unfinished[i].Status == "downloading" {
			s.Unfinished[i].Status = "pending"
		}
	}
	s.Phase = "idle"
	if err != nil {
		if ctx.Err() != nil {
			s.Phase = "cancelled"
			s.Error = "Stopped; the next sync will check again"
		} else {
			s.Phase = "error"
			s.Error = err.Error()
		}
	}
	if saveErr := e.persistLocked(); saveErr != nil {
		s.Phase = "error"
		s.Error = "Cannot save sync state: " + saveErr.Error()
	}
	e.cancel()
	e.cancel = nil
	close(e.done)
	e.mu.Unlock()
}

// Enumerate follows both folders and opaque continuation tokens, never leaving
// the configured prefix. Totals are published only after a complete listing.
func (e *Engine) enumerate(ctx context.Context) ([]storage.Object, error) {
	if !strings.HasPrefix(Prefix, e.cfg.Prefix) {
		return nil, errors.New("Bucket-root sync/ is outside the configured prefix")
	}
	folders := []string{Prefix}
	seenFolders := map[string]bool{Prefix: true}
	objects := map[string]storage.Object{}
	for i := 0; i < len(folders); i++ {
		prefix := folders[i]
		marker := ""
		seenMarkers := map[string]bool{}
		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			page, err := e.store.List(ctx, prefix, marker)
			if err != nil {
				return nil, err
			}
			for _, o := range page.Keys {
				if strings.HasPrefix(o.Key, prefix) && len(o.Key) <= 4096 && e.cfg.Supports(o.Key) && !strings.HasSuffix(o.Key, "/") {
					objects[o.Key] = o
				}
			}
			for _, f := range page.Folders {
				if f != prefix && strings.HasPrefix(f, prefix) && strings.HasSuffix(f, "/") && !seenFolders[f] {
					seenFolders[f] = true
					folders = append(folders, f)
				}
			}
			if len(objects)+len(folders) > 20000 {
				return nil, errors.New("sync/ exceeds 20000 files or folders; narrow the folder")
			}
			if !page.HasMore && page.NextMarker == "" {
				break
			}
			if page.NextMarker == "" || seenMarkers[page.NextMarker] {
				return nil, errors.New("Remote pagination marker repeated or missing; try again")
			}
			marker = page.NextMarker
			seenMarkers[marker] = true
		}
	}
	out := make([]storage.Object, 0, len(objects))
	for _, o := range objects {
		out = append(out, o)
	}
	return out, nil
}
func sortItems(items []Item) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Modified == items[j].Modified {
			return items[i].Key < items[j].Key
		}
		return items[i].Modified > items[j].Modified
	})
}
func counts(s *Snapshot, books []Item) {
	s.Known = true
	s.Fresh = true
	s.Books = append([]Item{}, books...)
	s.Unfinished = []Item{}
	for _, item := range books {
		if item.Status != "downloaded" {
			s.Unfinished = append(s.Unfinished, item)
		}
	}
	s.Total = len(books)
	s.Pending = len(s.Unfinished)
	s.Downloaded = s.Total - s.Pending
	s.Updated = time.Now().Unix()
}
func (e *Engine) scan(ctx context.Context, fetch bool) error {
	objects, err := e.enumerate(ctx)
	if err != nil {
		return err
	}
	books := make([]Item, 0, len(objects))
	for _, o := range objects {
		if err = ctx.Err(); err != nil {
			return err
		}
		meta, headErr := e.head(ctx, o.Key)
		item := Item{Key: o.Key, Modified: o.Modified, Size: o.Size, Status: "pending", Reason: "Not downloaded"}
		if headErr != nil {
			item.Status = "failed"
			item.Reason = "Check failed: " + headErr.Error()
		} else {
			item.Size = meta.Size
			if meta.Modified > 0 {
				item.Modified = meta.Modified
			}
			current, reason, checkErr := e.current(ctx, o.Key, meta)
			if checkErr != nil {
				return checkErr
			}
			if current {
				item.Status = "downloaded"
			} else if reason != "Not downloaded" {
				item.Status = "update"
			}
			item.Reason = reason
		}
		books = append(books, item)
	}
	sortItems(books)
	if err = e.publish(func(s *Snapshot) {
		counts(s, books)
		s.Skipped = s.Downloaded
		if fetch {
			s.Phase = "downloading"
		}
	}); err != nil {
		return err
	}
	if !fetch {
		return nil
	}
	var failures int
	// Work in modification order but keep failed entries in the unfinished list.
	for i := range books {
		if books[i].Status == "downloaded" {
			continue
		}
		item := books[i]
		if err = ctx.Err(); err != nil {
			return err
		}
		books[i].Status = "downloading"
		if err = e.publish(func(s *Snapshot) {
			s.Current = item.Key
			counts(s, books)
		}); err != nil {
			return err
		}
		fetched, fetchErr := e.fetch(ctx, item.Key)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if fetchErr != nil {
			failures++
			books[i].Status = "failed"
			books[i].Reason = fetchErr.Error()
		} else {
			books[i].Status = "downloaded"
			books[i].Reason = ""
		}
		if err = e.publish(func(s *Snapshot) {
			counts(s, books)
			if fetched {
				s.Fetched++
			}
		}); err != nil {
			return err
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d files incomplete; see ERR entries under All books and retry syncing", failures)
	}
	return nil
}
