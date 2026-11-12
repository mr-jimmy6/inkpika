package config

import (
	"strings"
	"testing"
)

func validS3() Config {
	return Config{Provider: S3, Bucket: "books", AccessKeyID: "fixture-id", SecretAccessKey: "fixture-secret", Listen: "127.0.0.1:8787", StateDir: "/state", DownloadDir: "/books", MaxDownloadMB: 512}
}

func TestS3PresetsAndAddresses(t *testing.T) {
	for _, tc := range []struct{ preset, region, endpoint, wantRegion, wantEndpoint, addressing string }{
		{"aws", "us-east-1", "", "us-east-1", "https://s3.us-east-1.amazonaws.com", "virtual"},
		{"aws", "cn-north-1", "", "cn-north-1", "https://s3.cn-north-1.amazonaws.com.cn", "virtual"},
		{"r2", "", "https://account.r2.cloudflarestorage.com/", "auto", "https://account.r2.cloudflarestorage.com", "path"},
		{"b2", "us-west-004", "", "us-west-004", "https://s3.us-west-004.backblazeb2.com", "path"},
		{"spaces", "nyc3", "", "nyc3", "https://nyc3.digitaloceanspaces.com", "virtual"},
		{"custom", "", "https://192.168.1.20:9443", "us-east-1", "https://192.168.1.20:9443", "path"},
		{"", "garage", "https://nas.example", "garage", "https://nas.example", "path"},
		{"custom", "", "https://books.example", "us-east-1", "https://books.example", "path"},
	} {
		t.Run(tc.preset+tc.region, func(t *testing.T) {
			c := validS3()
			c.S3Preset, c.Region, c.Endpoint = tc.preset, tc.region, tc.endpoint
			if err := c.Validate(); err != nil {
				t.Fatal(err)
			}
			if c.Region != tc.wantRegion || c.Endpoint != tc.wantEndpoint || c.S3Addressing != tc.addressing || c.S3Preset == "" {
				t.Fatal("incorrect preset")
			}
		})
	}
	c := validS3()
	c.S3Preset = "aws"
	c.Region = "us-east-1"
	c.Bucket = "books.example"
	if err := c.Validate(); err != nil || c.S3Addressing != "path" {
		t.Fatal("dotted bucket needs path-style TLS", err)
	}
}

func TestS3ValidationAndIdentity(t *testing.T) {
	base := validS3()
	base.S3Preset = "custom"
	base.Endpoint = "https://nas.example:9443"
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Config){
		func(c *Config) { c.Endpoint = "http://nas.example" },
		func(c *Config) { c.Endpoint = "https://nas.example/books" },
		func(c *Config) { c.Endpoint = "https://reader:secret@nas.example" },
		func(c *Config) { c.Endpoint = "https://nas.example?secret=x" },
		func(c *Config) { c.S3Preset = "unknown" },
		func(c *Config) { c.S3Preset = "r2" },
		func(c *Config) { c.Endpoint = "" },
		func(c *Config) { c.Zone = "pek3b" },
		func(c *Config) { c.Bucket = "127.0.0.1" },
		func(c *Config) { c.Bucket = "books..test" },
		func(c *Config) { c.Bucket = "books.-test" },
		func(c *Config) { c.Bucket = "books.test"; c.S3Addressing = "virtual" },
		func(c *Config) { c.Endpoint = "https://192.168.1.2:9443"; c.S3Addressing = "virtual" },
		func(c *Config) { c.S3Addressing = "invalid" },
		func(c *Config) { c.CABundle = "relative.pem" },
		func(c *Config) { c.SecurityToken = "bad\nheader" },
	} {
		c := base
		change(&c)
		if c.Validate() == nil {
			t.Fatal("invalid S3 config accepted")
		}
	}
	c := base
	c.SOCKS5Proxy = "socks5://reader:password@proxy.example:1080"
	c.CABundle = "/certificates/nas.pem"
	c.SecretAccessKey = "rotated"
	c.SecurityToken = "temporary"
	c.S3Addressing = "virtual"
	c.S3Preset = "aws"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.SourceIdentity() != base.SourceIdentity() {
		t.Fatal("transport, preset or key rotation changed object identity")
	}
	for _, change := range []func(*Config){func(c *Config) { c.Bucket = "other-books" }, func(c *Config) { c.Endpoint = "https://other.example" }, func(c *Config) { c.Region = "another-region" }} {
		c := base
		change(&c)
		if c.SourceIdentity() == base.SourceIdentity() {
			t.Fatal("different source shared identity")
		}
	}
}

func TestSOCKS5ValidationNeverEchoesCredentials(t *testing.T) {
	for _, good := range []string{"", "socks5://127.0.0.1:1080", "socks5h://reader:p%40ss%3Aword@proxy.example:1080", "socks5://[::1]:1080"} {
		if _, err := ParseSOCKS5Proxy(good); err != nil {
			t.Fatal(err)
		}
	}
	for _, bad := range []string{"http://reader:SECRET@proxy:1080", "socks5://proxy", "socks5://proxy:0", "socks5://proxy:65536", "socks5://proxy:abc", "socks5://proxy:1080/path", "socks5://proxy:1080?secret=SECRET", "socks5://proxy:1080#SECRET", "socks5://reader@proxy:1080", "socks5://reader:@proxy:1080", "socks5://:SECRET@proxy:1080", "socks5://reader:SECRET%0A@proxy:1080", "socks5://reader:" + strings.Repeat("x", 256) + "@proxy:1080"} {
		_, err := ParseSOCKS5Proxy(bad)
		if err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatal("invalid proxy accepted or secret exposed", err)
		}
	}
}
