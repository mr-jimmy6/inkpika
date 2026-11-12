package web

import (
	"github.com/mr-jimmy6/inkpika/internal/config"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLanguageIsOwnedByDeviceConfiguration(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		handler := New(config.Config{Language: lang, Listen: "127.0.0.1:8787"}, nil, nil, "session")
		for _, path := range []string{"/", "/i18n.js", "/api/info"} {
			r := httptest.NewRequest("GET", "http://127.0.0.1:8787"+path+"?lang=fr", nil)
			r.Header.Set("Accept-Language", "fr,zh;q=0.9,en;q=0.8")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Header().Get("Content-Language") != lang {
				t.Fatal(w.Header())
			}
			expected := "Cloud library"
			if lang == "zh" {
				expected = "云端书架"
			}
			if path == "/" && !strings.Contains(w.Body.String(), expected) {
				t.Fatal(w.Body.String())
			}
			if path == "/api/info" && (w.Code != 401 || !strings.Contains(w.Body.String(), expectedForSession(lang))) {
				t.Fatal(w.Code, w.Body.String())
			}
		}
	}
}
func expectedForSession(language string) string {
	if language == "zh" {
		return "会话已失效"
	}
	return "Session expired"
}
