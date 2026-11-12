// Package providers is the only place that chooses a concrete storage provider.
package providers

import (
	"fmt"

	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/providers/aliyunoss"
	"github.com/mr-jimmy6/inkpika/internal/providers/azureblob"
	"github.com/mr-jimmy6/inkpika/internal/providers/qingstor"
	"github.com/mr-jimmy6/inkpika/internal/providers/s3"
	"github.com/mr-jimmy6/inkpika/internal/storage"
)

func New(c config.Config) (storage.Store, error) {
	switch c.Provider {
	case config.QingStor:
		return qingstor.New(c)
	case config.AliyunOSS:
		return aliyunoss.New(c)
	case config.S3:
		return s3.New(c)
	case config.AzureBlob:
		return azureblob.New(c)
	default:
		return nil, fmt.Errorf("Unsupported storage backend: %s", c.Provider)
	}
}
