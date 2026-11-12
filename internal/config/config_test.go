package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigDefaultsAndRejections(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	base := `{"provider":"qingstor","bucket":"books","zone":"pek3b","access_key_id":"id","secret_access_key":"secret"}`
	if err := os.WriteFile(file, []byte(base), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if c.Endpoint != "https://pek3b.qingstor.com" || c.Listen != "127.0.0.1:8787" || !filepath.IsAbs(c.StateDir) {
		t.Fatal(c.Endpoint)
	}
	for _, change := range []func(*Config){func(c *Config) { c.Endpoint = "http://pek3b.qingstor.com" }, func(c *Config) { c.Listen = "0.0.0.0:8787" }, func(c *Config) { c.Endpoint = "https://books.pek3b.qingstor.com" }, func(c *Config) { c.DownloadDir = "../books" }, func(c *Config) { c.SecretAccessKey = "" }} {
		bad := c
		change(&bad)
		if err = bad.Validate(); err == nil {
			t.Fatal("invalid config accepted")
		}
	}
	if err = os.WriteFile(file, []byte(base+` {}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(file); err == nil {
		t.Fatal("trailing object accepted")
	}
}

func TestProviderConfigAndStateIsolation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	load := func(values map[string]interface{}) (Config, error) {
		t.Helper()
		data, err := json.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
		return Load(file)
	}
	values := map[string]interface{}{"provider": "aliyun_oss", "region": "cn-hangzhou", "bucket": "books", "access_key_id": "id", "secret_access_key": "secret", "security_token": "temporary"}
	a, err := load(values)
	if err != nil {
		t.Fatal(err)
	}
	if a.Endpoint != "https://oss-cn-hangzhou.aliyuncs.com" || a.DownloadDir != "/mnt/us/documents/InkPika" || !strings.HasPrefix(filepath.Base(a.StateDir), "aliyun_oss-") {
		t.Fatalf("bad OSS config: %s %s %s", a.Endpoint, a.DownloadDir, a.StateDir)
	}
	values["secret_access_key"] = "rotated"
	b, err := load(values)
	if err != nil || a.StateDir != b.StateDir || a.SourceIdentity() != b.SourceIdentity() {
		t.Fatal("key rotation discarded download identity", err)
	}
	values["bucket"] = "other-books"
	b, err = load(values)
	if err != nil || a.StateDir == b.StateDir {
		t.Fatal("different buckets shared state", err)
	}
	values["provider"] = "qingstor"
	values["region"] = "pek3b"
	delete(values, "security_token")
	b, err = load(values)
	if err != nil || a.StateDir == b.StateDir {
		t.Fatal("different providers shared state", err)
	}
	values["provider"] = "invalid"
	if _, err = load(values); err == nil {
		t.Fatal("unknown provider accepted")
	}
}

func TestOSSConfigRejections(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	for _, suffix := range []string{`"region":""`, `"region":"oss-cn-hangzhou"`, `"region":"cn-hangzhou","zone":"pek3b"`, `"region":"cn-hangzhou","endpoint":"https://books.oss-cn-hangzhou.aliyuncs.com"`, `"region":"cn-hangzhou","security_token":"bad\nheader"`} {
		data := `{"provider":"aliyun_oss","bucket":"books","access_key_id":"id","secret_access_key":"secret",` + suffix + `}`
		if err := os.WriteFile(file, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(file); err == nil {
			t.Fatal("bad OSS config accepted: " + suffix)
		}
	}
}
