package i18n

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeviceLocaleAndFallback(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"LANG=zh_CN.utf8\n", "zh"}, {"export LANG='zh_TW.UTF-8'\n", "zh"}, {"zh-Hans-CN", "zh"}, {"zh-Hant", "zh"}, {"en_GB.utf8", "en"}, {"LANG=ja_JP.utf8", "en"}, {"", "en"}, {"not-a-locale", "en"},
	} {
		if got := Locale(tc.input); got != tc.want {
			t.Errorf("Locale(%q) = %s, want %s", tc.input, got, tc.want)
		}
	}
	file := filepath.Join(t.TempDir(), "locale")
	if err := os.WriteFile(file, []byte("LANG=zh_CN.utf8\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ preference, want string }{{"", "zh"}, {"auto", "zh"}, {"en", "en"}, {"zh", "zh"}, {"fr", "en"}} {
		if got := ResolveFile(tc.preference, file); got != tc.want {
			t.Errorf("%q: %s", tc.preference, got)
		}
	}
	t.Setenv("LANG", "zh_CN.UTF-8")
	if ResolveFile("auto", file+".missing") != "en" {
		t.Fatal("missing device locale used the host locale")
	}
	if err := os.WriteFile(file, []byte(strings.Repeat("zh_CN\n", 1024)), 0600); err != nil {
		t.Fatal(err)
	}
	if ResolveFile("auto", file) != "en" || ResolveFile("zh", file) != "zh" {
		t.Fatal("bounded read or explicit override failed")
	}
}

func TestPreferenceDoesNotRequireCloudCredentials(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	for _, tc := range []struct{ data, want string }{
		{`{"language":"zh"}`, "zh"}, {`{"language":"auto","provider":"invalid"}`, "auto"}, {`{}`, "auto"}, {`{"language":"fr"}`, "en"}, {`{"language":"zh"} {}`, "en"}, {`{"language":`, "en"},
	} {
		if err := os.WriteFile(file, []byte(tc.data), 0600); err != nil {
			t.Fatal(err)
		}
		if got := Preference(file); got != tc.want {
			t.Fatalf("%s: %s", tc.data, got)
		}
	}
	if Preference(file+".missing") != "auto" {
		t.Fatal("missing config must follow device")
	}
}

func TestMessagesPreserveArgumentsAndLegacyState(t *testing.T) {
	if got := Text("zh", "File exceeds the configured 42 MB limit"); got != "文件超过配置的 42 MB 大小上限" {
		t.Fatal(got)
	}
	if got := Text("en", "文件超过配置的 42 MB 大小上限"); got != "File exceeds the configured 42 MB limit" {
		t.Fatal(got)
	}
	if got := Text("zh", `Unsupported provider "Downloading"; explicitly set aliyun_oss, qingstor, s3 or azure_blob`); !strings.Contains(got, `"Downloading"`) {
		t.Fatal("argument was translated", got)
	}
	if got := Text("zh", "Cannot save sync state: Service stopped"); got != "保存同步状态失败: 服务已停止" {
		t.Fatal(got)
	}
	if got := Text("zh", "unrecognized system error"); got != "unrecognized system error" {
		t.Fatal(got)
	}
}

func TestResponseLocalizesOnlyHumanFields(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("Content-Language", "zh")
	JSON(w, 200, map[string]interface{}{"error": "Service stopped", "key": "Service stopped", "name": "Downloading", "status": "downloading", "token": "Ready", "size": int64(9007199254740993), "books": []map[string]string{{"reason": "Not downloaded", "key": "Not downloaded"}}})
	var got map[string]interface{}
	d := json.NewDecoder(w.Body)
	d.UseNumber()
	if err := d.Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["error"] != "服务已停止" || got["key"] != "Service stopped" || got["name"] != "Downloading" || got["status"] != "downloading" || got["token"] != "Ready" || got["size"] != json.Number("9007199254740993") {
		t.Fatal(got)
	}
	book := got["books"].([]interface{})[0].(map[string]interface{})
	if book["key"] != "Not downloaded" || book["reason"] != "未下载" {
		t.Fatal(book)
	}
}

func TestStaticHTMLAndScriptLanguage(t *testing.T) {
	page := []byte(`<html lang="en"><title>Wireless transfer</title><button>Connect</button><input placeholder="e.g. books/"><script src="/app.js"></script></html>`)
	zh := string(HTML(page, "zh"))
	if !strings.Contains(zh, `lang="zh-CN"`) || !strings.Contains(zh, ">无线传书<") || !strings.Contains(zh, `src="/app.js"`) {
		t.Fatal(zh)
	}
	if !strings.Contains(string(Script("en")), `window.InkPikaLocale = "en";`) {
		t.Fatal("device script must pin its language")
	}
	if strings.Contains(string(Script("")), `window.InkPikaLocale =`) {
		t.Fatal("static website must use its explicit switch")
	}
}
