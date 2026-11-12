package config

import (
	"encoding/base64"
	"testing"
)

func TestAdditionalProviderDefaultsAndRules(t *testing.T) {
	for _, tc := range []struct{ preset, region, bucket, host, address string }{
		{"tos", "cn-beijing", "books", "https://tos-s3-cn-beijing.volces.com", "virtual"},
		{"ibm", "us-south", "books", "https://s3.us-south.cloud-object-storage.appdomain.cloud", "path"},
		{"ibm", "eu", "books", "https://s3.eu.cloud-object-storage.appdomain.cloud", "path"},
		{"gcs", "", "books.example", "https://storage.googleapis.com", "path"},
		{"cos", "ap-guangzhou", "books-1250000000", "https://cos.ap-guangzhou.myqcloud.com", "virtual"},
	} {
		t.Run(tc.preset+tc.region, func(t *testing.T) {
			c := validS3()
			c.S3Preset = tc.preset
			c.Region = tc.region
			c.Bucket = tc.bucket
			if err := c.Validate(); err != nil {
				t.Fatal(err)
			}
			if c.Endpoint != tc.host || c.S3Addressing != tc.address {
				t.Fatal("wrong derived configuration")
			}
			if tc.preset == "gcs" && c.Region != "auto" {
				t.Fatal("GCS default region")
			}
			bad := c
			if tc.preset == "tos" || tc.preset == "cos" {
				bad.S3Addressing = "path"
			} else {
				bad.SecurityToken = "unsupported-session-token"
			}
			if bad.Validate() == nil {
				t.Fatal("incompatible option accepted")
			}
			if tc.preset == "cos" {
				bad = c
				bad.Bucket = "books"
				if bad.Validate() == nil {
					t.Fatal("missing APPID accepted")
				}
			}
		})
	}
}

func TestAzureValidationAndIdentity(t *testing.T) {
	c := validS3()
	c.Provider = AzureBlob
	c.AccessKeyID = "bookaccount"
	c.SecretAccessKey = base64.StdEncoding.EncodeToString(make([]byte, 64))
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.Endpoint != "https://bookaccount.blob.core.windows.net" || c.Region != "" {
		t.Fatal("wrong Azure defaults")
	}
	for _, change := range []func(*Config){
		func(c *Config) { c.AccessKeyID = "UpperCase" }, func(c *Config) { c.AccessKeyID = "ab" },
		func(c *Config) { c.SecretAccessKey = "not-base64" }, func(c *Config) { c.SecretAccessKey = base64.StdEncoding.EncodeToString([]byte("short")) },
		func(c *Config) { c.SecurityToken = "unsupported" }, func(c *Config) { c.Zone = "legacy" },
		func(c *Config) { c.Bucket = "two--hyphens" }, func(c *Config) { c.Bucket = "dot.name" },
		func(c *Config) { c.S3Preset = "aws" }, func(c *Config) { c.Endpoint = "https://bookaccount.blob.core.windows.net/books" },
	} {
		bad := c
		change(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid Azure configuration accepted")
		}
	}
	same := c
	same.SecretAccessKey = base64.StdEncoding.EncodeToString([]byte("0123456789012345678901234567890123456789012345678901234567890123456789")[:64])
	same.Region = "unused"
	if err := same.Validate(); err != nil || same.SourceIdentity() != c.SourceIdentity() {
		t.Fatal("rotation changed source identity", err)
	}
	same.Endpoint = "https://bookaccount.blob.core.chinacloudapi.cn"
	if err := same.Validate(); err != nil || same.SourceIdentity() == c.SourceIdentity() {
		t.Fatal("sovereign endpoint failed", err)
	}
}
