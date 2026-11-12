package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	QingStor  = "qingstor"
	AliyunOSS = "aliyun_oss"
	S3        = "s3"
	AzureBlob = "azure_blob"
)

var s3Bucket = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*[a-z0-9]$`)
var azureAccount = regexp.MustCompile(`^[a-z0-9]{3,24}$`)
var cosBucket = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*-[0-9]+$`)

// validateProvider owns endpoint derivation and provider-specific configuration.
// All callers must select a provider explicitly.
func (c *Config) validateProvider() error {
	if c.Provider != S3 && (c.S3Preset != "" || c.S3Addressing != "") {
		return errors.New("s3_preset and s3_addressing apply only to S3 storage")
	}
	switch c.Provider {
	case AzureBlob:
		if c.Zone != "" || c.SecurityToken != "" {
			return errors.New("Azure Blob uses an account name and account key; do not set zone or security_token")
		}
		if !azureAccount.MatchString(c.AccessKeyID) {
			return errors.New("Azure storage account names must contain 3–24 lowercase letters or digits")
		}
		key, err := base64.StdEncoding.DecodeString(c.SecretAccessKey)
		if err != nil || len(key) != 64 {
			return errors.New("Use the Base64-encoded Azure account key from the console")
		}
		if strings.Contains(c.Bucket, "--") {
			return errors.New("Azure container names cannot contain consecutive hyphens")
		}
		c.Region = "" // Azure Shared Key does not use a signing region.
		if c.Endpoint == "" {
			c.Endpoint = "https://" + c.AccessKeyID + ".blob.core.windows.net"
		}
	case QingStor:
		if c.SecurityToken != "" {
			return errors.New("QingStor does not support security_token; temporary tokens apply only to Alibaba Cloud OSS or S3")
		}
		if c.Region != "" && c.Zone != "" && c.Region != c.Zone {
			return errors.New("QingStor region and the zone field must match")
		}
		if c.Region == "" {
			c.Region = c.Zone
		}
		c.Zone = c.Region
		if c.Endpoint == "" {
			if !label.MatchString(c.Region) {
				return errors.New("Enter a valid QingStor region (or zone), such as pek3b")
			}
			c.Endpoint = "https://" + c.Region + ".qingstor.com"
		}
	case AliyunOSS:
		if !label.MatchString(c.Region) || strings.HasPrefix(c.Region, "oss-") {
			return errors.New("Alibaba Cloud OSS requires a V4 signing region, such as cn-hangzhou, without the oss- prefix")
		}
		if c.Zone != "" {
			return errors.New("For Alibaba Cloud OSS, use region and remove the QingStor zone field")
		}
		if c.Endpoint == "" {
			c.Endpoint = "https://oss-" + c.Region + ".aliyuncs.com"
		}
	case S3:
		if c.Zone != "" {
			return errors.New("For S3, use region and remove the QingStor zone field")
		}
		if c.S3Preset == "" {
			c.S3Preset = "custom"
		}
		switch c.S3Preset {
		case "aws", "b2", "spaces", "tos", "ibm", "cos":
			if !label.MatchString(c.Region) {
				return errors.New("Enter the bucket's actual region for the selected platform")
			}
			if c.Endpoint == "" {
				switch c.S3Preset {
				case "aws":
					c.Endpoint = "https://s3." + c.Region + ".amazonaws.com"
					if strings.HasPrefix(c.Region, "cn-") {
						c.Endpoint += ".cn"
					}
				case "b2":
					c.Endpoint = "https://s3." + c.Region + ".backblazeb2.com"
				case "spaces":
					c.Endpoint = "https://" + c.Region + ".digitaloceanspaces.com"
				case "tos":
					c.Endpoint = "https://tos-s3-" + c.Region + ".volces.com"
				case "ibm":
					c.Endpoint = "https://s3." + c.Region + ".cloud-object-storage.appdomain.cloud"
				case "cos":
					c.Endpoint = "https://cos." + c.Region + ".myqcloud.com"
				}
			}
		case "r2":
			if c.Region == "" {
				c.Region = "auto"
			}
			if c.Region != "auto" {
				return errors.New("Cloudflare R2 signing region must be auto")
			}
		case "gcs":
			if c.Region == "" {
				c.Region = "auto"
			}
			if !label.MatchString(c.Region) {
				return errors.New("Google Cloud Storage signing region must be auto or a valid region")
			}
			if c.Endpoint == "" {
				c.Endpoint = "https://storage.googleapis.com"
			}
		case "custom":
			if c.Region == "" {
				c.Region = "us-east-1"
			}
			if !label.MatchString(c.Region) {
				return errors.New("Enter the signing region configured on the S3 server")
			}
		default:
			return errors.New("s3_preset must be aws, r2, b2, spaces, tos, ibm, gcs, cos or custom")
		}
		if c.Endpoint == "" {
			return errors.New("R2 and custom S3 require an HTTPS endpoint without the bucket")
		}
		if (c.S3Preset == "ibm" || c.S3Preset == "gcs") && c.SecurityToken != "" {
			return errors.New("Use HMAC credentials for IBM COS and Google Cloud Storage; do not set security_token")
		}
		if c.S3Preset == "cos" && !cosBucket.MatchString(c.Bucket) {
			return errors.New("Tencent COS bucket names must include the APPID suffix, such as books-1250000000")
		}
		if c.S3Addressing == "" {
			c.S3Addressing = "path"
			if c.S3Preset == "tos" || c.S3Preset == "cos" || ((c.S3Preset == "aws" || c.S3Preset == "spaces") && !strings.Contains(c.Bucket, ".")) {
				c.S3Addressing = "virtual"
			}
		}
		if (c.S3Preset == "tos" || c.S3Preset == "cos") && c.S3Addressing != "virtual" {
			return errors.New("Use virtual-hosted addressing for Volcengine TOS and Tencent COS")
		}
		if c.S3Addressing != "path" && c.S3Addressing != "virtual" {
			return errors.New("s3_addressing must be path or virtual")
		}
	default:
		return fmt.Errorf("Unsupported provider %q; explicitly set aliyun_oss, qingstor, s3 or azure_blob", c.Provider)
	}
	return nil
}

func (c Config) ProviderLabel() string {
	switch c.Provider {
	case AliyunOSS:
		return "Alibaba Cloud OSS"
	case QingStor:
		return "QingStor"
	case AzureBlob:
		return "Microsoft Azure Blob Storage"
	case S3:
		switch c.S3Preset {
		case "aws":
			return "Amazon S3"
		case "r2":
			return "Cloudflare R2"
		case "b2":
			return "Backblaze B2"
		case "spaces":
			return "DigitalOcean Spaces"
		case "tos":
			return "Volcengine TOS"
		case "ibm":
			return "IBM Cloud Object Storage"
		case "gcs":
			return "Google Cloud Storage"
		case "cos":
			return "Tencent COS"
		default:
			return "S3-compatible storage / NAS"
		}
	default:
		return c.Provider
	}
}
