package bookformat

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSharedRulesAndDefaults(t *testing.T) {
	values, err := Normalize([]string{".PDF", " azw3 ", "pdf", "epub"})
	if err != nil || !reflect.DeepEqual(values, []string{"pdf", "azw3", "epub"}) {
		t.Fatal(values, err)
	}
	if !Allows(values, "目录/Book.EPUB") || Allows(values, "book.txt") || Allows(nil, "book.epub") || !Allows(nil, "book.azw3") {
		t.Fatal("format allowlist mismatch")
	}
	for _, bad := range []string{"*", "../pdf", "pdf,txt", "", "x<script>"} {
		if _, err := Normalize([]string{bad}); err == nil {
			t.Fatal("invalid format allowed", bad)
		}
	}
	f := filepath.Join(t.TempDir(), "config.json")
	for _, raw := range []string{`{}`, `{"ebook_formats":[]}`} {
		os.WriteFile(f, []byte(raw), 0600)
		formats, err := FromFile(f)
		if err != nil || !reflect.DeepEqual(formats, Defaults()) {
			t.Fatal(formats, err)
		}
	}
	os.WriteFile(f, []byte(`{"ebook_formats":["pdf"]}`), 0600)
	formats, err := FromFile(f)
	if err != nil || Allows(formats, "book.azw3") || !Allows(formats, "book.pdf") {
		t.Fatal(formats, err)
	}
	os.WriteFile(f, []byte(`{"ebook_formats":["*"]}`), 0600)
	if _, err := FromFile(f); err == nil {
		t.Fatal("bad config widened allowlist")
	}
}
