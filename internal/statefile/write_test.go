package statefile

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteReplacesWithoutFollowingStaleTempSymlink(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "state.json")
	unrelated := filepath.Join(dir, "unrelated")
	if err := os.WriteFile(unrelated, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(unrelated, filename+".tmp"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"old", "new"} {
		if err := Write(filename, []byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != "new" {
		t.Fatalf("replacement: %q, %v", data, err)
	}
	data, err = os.ReadFile(unrelated)
	if err != nil || string(data) != "keep" {
		t.Fatalf("unrelated file changed: %q, %v", data, err)
	}
}

func TestFailedReplacementPreservesDestinationAndCleansTemp(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "state.json")
	if err := os.Mkdir(filename, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(filename, "keep")
	if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Write(filename, []byte("new")); err == nil {
		t.Fatal("replaced a directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files leaked: %v, %v", entries, err)
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "keep" {
		t.Fatalf("destination changed: %q, %v", data, err)
	}
}

func TestConcurrentWritesPublishWholeFiles(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "state.json")
	var wg sync.WaitGroup
	for i := byte(0); i < 8; i++ {
		wg.Add(1)
		go func(value byte) {
			defer wg.Done()
			if err := Write(filename, bytes.Repeat([]byte{value}, 8192)); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 8192 || !bytes.Equal(data, bytes.Repeat(data[:1], 8192)) {
		t.Fatal("mixed or partial contents")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files leaked: %v, %v", entries, err)
	}
}
