package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/mr-jimmy6/inkpika/internal/bookformat"
	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/statefile"
)

type Store interface {
	Head(context.Context, string) (*http.Response, error)
	Get(context.Context, string, int64, string) (*http.Response, error)
}

type Job struct {
	ID               string `json:"id"`
	Key              string `json:"key"`
	Name             string `json:"name"`
	Status           string `json:"status"`
	Size             int64  `json:"size"`
	Downloaded       int64  `json:"downloaded"`
	Error            string `json:"error,omitempty"`
	ETag             string `json:"etag,omitempty"`
	Updated          int64  `json:"updated"`
	TransferComplete bool   `json:"transfer_complete,omitempty"`
}

type Manager struct {
	mu           sync.Mutex
	cfg          config.Config
	store        Store
	jobs         []*Job
	wake         chan struct{}
	ctx          context.Context
	stop         context.CancelFunc
	activeID     string
	activeCancel context.CancelFunc
	done         chan struct{}
	transferSlot chan struct{}
}

func Supported(key string) bool {
	return bookformat.Allows(nil, key)
}

func safeName(key, id string) string {
	ext := strings.ToLower(path.Ext(key))
	base := strings.TrimSuffix(path.Base(key), path.Ext(key))
	base = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, base)
	runes := []rune(strings.Trim(base, ". "))
	if len(runes) > 55 {
		runes = runes[:55]
	}
	base = string(runes)
	if base == "" {
		base = "book"
	}
	return base + "-" + id[:12] + ext
}

func New(c config.Config, store Store) (*Manager, error) {
	for _, d := range []string{c.DownloadDir, filepath.Join(c.StateDir, "parts")} {
		if err := os.MkdirAll(d, 0700); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{cfg: c, store: store, wake: make(chan struct{}, 1), ctx: ctx, stop: cancel, done: make(chan struct{}), transferSlot: make(chan struct{}, 1), jobs: []*Job{}}
	data, err := os.ReadFile(filepath.Join(c.StateDir, "jobs.json"))
	if err == nil {
		if err = json.Unmarshal(data, &m.jobs); err != nil {
			cancel()
			return nil, fmt.Errorf("Download state is corrupt; back up state/jobs.json before removing it: %w", err)
		}
		for _, j := range m.jobs {
			if j == nil || j.ID != m.id(j.Key) || j.Name != safeName(j.Key, j.ID) {
				cancel()
				return nil, errors.New("Download state contains invalid filenames or belongs to another bucket; use a separate state_dir")
			}
			if j.Status == "downloading" || j.Status == "queued" {
				j.Status = "paused"
				j.Error = "Service restarted; resume the download"
			}
		}
	} else if !os.IsNotExist(err) {
		cancel()
		return nil, err
	}
	go m.run()
	return m, nil
}

func (m *Manager) id(key string) string {
	h := sha256.Sum256([]byte(m.cfg.SourceIdentity() + "\x00" + key))
	return hex.EncodeToString(h[:16])
}

// Target is shared by browser downloads and KUAL sync for the same source/key.
func (m *Manager) Target(key string) string {
	return filepath.Join(m.cfg.DownloadDir, safeName(key, m.id(key)))
}

// WithTransfer serializes file inspection/replacement with browser downloads.
func (m *Manager) WithTransfer(ctx context.Context, fn func() error) error {
	select {
	case m.transferSlot <- struct{}{}:
		defer func() { <-m.transferSlot }()
		if err := ctx.Err(); err != nil {
			return err
		}
		return fn()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsDownloaded checks the final file, independently of cleared download history.
// A local copy may be older than the current object; this is not a version check.
func (m *Manager) IsDownloaded(key string) bool {
	stat, err := os.Lstat(filepath.Join(m.cfg.DownloadDir, safeName(key, m.id(key))))
	return err == nil && stat.Mode().IsRegular()
}

// LocalState is checked on every history read, independently of the persisted
// transfer result. A completed transfer does not prove the file still exists.
func (m *Manager) LocalState(j Job) string {
	if j.Status != "completed" {
		return "pending"
	}
	stat, err := os.Lstat(m.Target(j.Key))
	if os.IsNotExist(err) {
		return "removed"
	}
	if err != nil {
		return "unavailable"
	}
	if !stat.Mode().IsRegular() || stat.Size() != j.Size {
		return "changed"
	}
	return "saved"
}

func (m *Manager) saveLocked() error {
	data, err := json.MarshalIndent(m.jobs, "", "  ")
	if err != nil {
		return err
	}
	return statefile.Write(filepath.Join(m.cfg.StateDir, "jobs.json"), data)
}

func (m *Manager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) Add(key string) (Job, error) {
	if !strings.HasPrefix(key, m.cfg.Prefix) || strings.HasSuffix(key, "/") || len(key) > 4096 || !m.cfg.Supports(key) {
		return Job{}, errors.New("File is outside the configured scope or its ebook format is not allowed")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.Key == key {
			if j.Status == "completed" && !m.IsDownloaded(key) {
				if _, err := os.Lstat(filepath.Join(m.cfg.DownloadDir, j.Name)); !os.IsNotExist(err) {
					return Job{}, errors.New("Cannot download again: target path already exists or is inaccessible")
				}
				previous := *j
				*j = Job{ID: j.ID, Key: j.Key, Name: j.Name, Status: "queued", Updated: time.Now().Unix()}
				if err := m.saveLocked(); err != nil {
					*j = previous
					return Job{}, err
				}
				m.signal()
			}
			return *j, nil
		}
	}
	if len(m.jobs) >= 200 {
		return Job{}, errors.New("Download history reached 200 entries; clear finished entries first")
	}
	id := m.id(key)
	j := &Job{ID: id, Key: key, Name: safeName(key, id), Status: "queued", Updated: time.Now().Unix()}
	if _, err := os.Lstat(filepath.Join(m.cfg.DownloadDir, j.Name)); err == nil {
		return Job{}, errors.New("Target file already exists and will not be overwritten; read it in the Kindle library")
	} else if !os.IsNotExist(err) {
		return Job{}, err
	}
	m.jobs = append(m.jobs, j)
	if err := m.saveLocked(); err != nil {
		m.jobs = m.jobs[:len(m.jobs)-1]
		return Job{}, err
	}
	m.signal()
	return *j, nil
}

func (m *Manager) Snapshot() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, len(m.jobs))
	for i, j := range m.jobs {
		out[i] = *j
	}
	return out
}

func (m *Manager) Action(id, action string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.ID == id {
			switch action {
			case "retry":
				if m.activeID == id {
					return errors.New("Task is pausing; resume it shortly")
				}
				if j.Status != "failed" && j.Status != "paused" && j.Status != "cancelled" {
					return errors.New("This task cannot be retried now")
				}
				j.Status = "queued"
				j.Error = ""
				m.signal()
			case "cancel":
				if j.Status != "queued" && j.Status != "downloading" {
					return errors.New("This task cannot be paused now")
				}
				j.Status = "cancelled"
				if m.activeID == id && m.activeCancel != nil {
					m.activeCancel()
				}
			default:
				return errors.New("Unknown action")
			}
			j.Updated = time.Now().Unix()
			return m.saveLocked()
		}
	}
	return errors.New("Download task does not exist")
}

// Clear removes finished history and partial files; it never removes completed books.
func (m *Manager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := make([]*Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		if j.Status == "queued" || j.Status == "downloading" || j.Status == "paused" || j.Status == "cancelled" || j.ID == m.activeID {
			kept = append(kept, j)
			continue
		}
		if err := os.Remove(filepath.Join(m.cfg.StateDir, "parts", j.ID+".part")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	m.jobs = kept
	return m.saveLocked()
}

func (m *Manager) Close() { m.stop(); <-m.done }

func (m *Manager) run() {
	defer close(m.done)
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.wake:
		}
		for m.ctx.Err() == nil {
			m.mu.Lock()
			var job *Job
			for _, j := range m.jobs {
				if j.Status == "queued" {
					job = j
					break
				}
			}
			if job == nil {
				m.mu.Unlock()
				break
			}
			ctx, cancel := context.WithCancel(m.ctx)
			m.activeID = job.ID
			m.activeCancel = cancel
			job.Status = "downloading"
			job.Error = ""
			snapshot := *job
			err := m.saveLocked()
			m.mu.Unlock()
			if err == nil {
				err = m.WithTransfer(ctx, func() error { return m.transfer(ctx, snapshot) })
			}
			cancel()
			m.mu.Lock()
			if err == nil {
				job.Status = "completed"
				job.Downloaded = job.Size
				job.Error = ""
			} else if job.Status != "cancelled" {
				if m.ctx.Err() != nil {
					job.Status = "paused"
					job.Error = "Service stopped; resume the download"
				} else {
					job.Status = "failed"
					job.Error = err.Error()
				}
			}
			job.Updated = time.Now().Unix()
			m.activeID = ""
			m.activeCancel = nil
			if saveErr := m.saveLocked(); saveErr != nil {
				job.Error = "Cannot save download history: " + saveErr.Error()
			}
			m.mu.Unlock()
		}
	}
}

func (m *Manager) update(id string, fn func(*Job)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.ID == id {
			fn(j)
			return m.saveLocked()
		}
	}
	return errors.New("Task does not exist")
}

func (m *Manager) transfer(ctx context.Context, j Job) error {
	meta, err := m.store.Head(ctx, j.Key)
	if err != nil {
		return err
	}
	if meta.Body != nil {
		meta.Body.Close()
	}
	total := meta.ContentLength
	etag := meta.Header.Get("ETag")
	if total < 0 {
		return errors.New("Server did not return file size; cannot download safely")
	}
	if total > m.cfg.MaxDownloadMB*1024*1024 {
		return fmt.Errorf("File exceeds the configured %d MB limit", m.cfg.MaxDownloadMB)
	}
	part := filepath.Join(m.cfg.StateDir, "parts", j.ID+".part")
	if stat, e := os.Lstat(part); e == nil && !stat.Mode().IsRegular() {
		return errors.New("Temporary file is not a regular file")
	}
	f, err := os.OpenFile(part, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	offset := stat.Size()
	if etag == "" || etag != j.ETag || total != j.Size || offset > total {
		if err = f.Truncate(0); err != nil {
			return err
		}
		offset = 0
	}
	if free, err := FreeBytes(m.cfg.StateDir); err != nil {
		return err
	} else if uint64(total-offset)+8*1024*1024 > free {
		return errors.New("Insufficient Kindle storage (8 MB additional reserve required)")
	}
	if err = m.update(j.ID, func(job *Job) {
		job.Size = total
		job.ETag = etag
		job.Downloaded = offset
		job.TransferComplete = false
	}); err != nil {
		return err
	}
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	if offset < total || total == 0 {
		// Cancel stalled bodies as well as slow headers. A blocked Read must not hold the queue forever.
		bodyCtx, bodyCancel := context.WithCancel(ctx)
		defer bodyCancel()
		idle := time.AfterFunc(60*time.Second, bodyCancel)
		defer idle.Stop()
		resp, err := m.store.Get(bodyCtx, j.Key, offset, etag)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if etag != "" && resp.Header.Get("ETag") != "" && resp.Header.Get("ETag") != etag {
			return errors.New("Remote file changed before download; try again")
		}
		if offset > 0 {
			if resp.StatusCode == http.StatusOK { // Some servers ignore Range: restart, never append a full response.
				if err = f.Truncate(0); err != nil {
					return err
				}
				if _, err = f.Seek(0, io.SeekStart); err != nil {
					return err
				}
				offset = 0
			} else if resp.StatusCode != http.StatusPartialContent || !validRange(resp.Header.Get("Content-Range"), offset, total) {
				return errors.New("Server resume response does not match; try again")
			}
		} else if resp.StatusCode != http.StatusOK {
			return errors.New("Server returned an incomplete download response")
		}
		if resp.ContentLength >= 0 && resp.ContentLength != total-offset {
			return errors.New("Server file length changed; try again")
		}
		pw := &progressWriter{writer: f, manager: m, id: j.ID, offset: offset, idle: idle, last: time.Now()}
		n, err := io.CopyBuffer(pw, io.LimitReader(resp.Body, total-offset+1), make([]byte, 32*1024))
		if saveErr := m.update(j.ID, func(job *Job) { job.Downloaded = offset + n }); saveErr != nil {
			return saveErr
		}
		if err != nil {
			if bodyCtx.Err() != nil && ctx.Err() == nil {
				return errors.New("No download data for 60 seconds; check the network, then resume")
			}
			return fmt.Errorf("Download interrupted; you can resume: %w", err)
		}
		if n != total-offset {
			return errors.New("File incomplete; temporary file preserved for resuming")
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err = m.update(j.ID, func(job *Job) { job.TransferComplete = true }); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	target := filepath.Join(m.cfg.DownloadDir, j.Name)
	if _, err = os.Lstat(target); err == nil {
		return errors.New("Target file already exists and will not be overwritten; check the Kindle library")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = os.Rename(part, target); err != nil {
		return fmt.Errorf("Cannot save book; state_dir and download_dir must be on the same filesystem: %w", err)
	}
	return nil
}

func validRange(value string, offset, total int64) bool {
	return value == "bytes "+strconv.FormatInt(offset, 10)+"-"+strconv.FormatInt(total-1, 10)+"/"+strconv.FormatInt(total, 10)
}

type progressWriter struct {
	writer  io.Writer
	manager *Manager
	id      string
	offset  int64
	idle    *time.Timer
	last    time.Time
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.writer.Write(b)
	p.offset += int64(n)
	p.idle.Reset(60 * time.Second)
	if time.Since(p.last) >= time.Second {
		p.manager.mu.Lock()
		for _, j := range p.manager.jobs {
			if j.ID == p.id {
				j.Downloaded = p.offset
			}
		}
		p.manager.mu.Unlock()
		p.last = time.Now()
	}
	return n, err
}
