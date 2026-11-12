package providers

import (
	"context"
	"encoding/base64"
	"io"
	"net"
	"testing"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/providers/aliyunoss"
	"github.com/mr-jimmy6/inkpika/internal/providers/azureblob"
	"github.com/mr-jimmy6/inkpika/internal/providers/qingstor"
	"github.com/mr-jimmy6/inkpika/internal/providers/s3"
)

func TestProviderSelection(t *testing.T) {
	for _, p := range []string{"", config.QingStor, config.AliyunOSS, config.S3, config.AzureBlob, "invalid"} {
		store, err := New(config.Config{Provider: p, SecretAccessKey: base64.StdEncoding.EncodeToString(make([]byte, 64))})
		switch p {
		case config.AzureBlob:
			if _, ok := store.(*azureblob.Client); err != nil || !ok {
				t.Fatal("Azure selection failed")
			}
		case config.QingStor:
			if _, ok := store.(*qingstor.Client); err != nil || !ok {
				t.Fatal("QingStor selection failed")
			}
		case config.AliyunOSS:
			if _, ok := store.(*aliyunoss.Client); err != nil || !ok {
				t.Fatal("OSS selection failed")
			}
		case config.S3:
			if _, ok := store.(*s3.Client); err != nil || !ok {
				t.Fatal("S3 selection failed")
			}
		default:
			if err == nil {
				t.Fatal("unknown provider silently selected")
			}
		}
	}
}

func TestEveryProviderRejectsInvalidProxyConfiguration(t *testing.T) {
	for _, provider := range []string{config.AliyunOSS, config.QingStor, config.S3, config.AzureBlob} {
		if _, err := New(config.Config{Provider: provider, SOCKS5Proxy: "http://invalid-proxy:1080"}); err == nil {
			t.Fatal("provider bypasses configured transport", provider)
		}
	}
}

func TestCancelledSOCKSHandshakeIsClosedByEveryProvider(t *testing.T) {
	for _, provider := range []string{config.AliyunOSS, config.QingStor, config.S3, config.AzureBlob} {
		t.Run(provider, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			connected, closed := make(chan struct{}), make(chan struct{})
			go func() {
				defer close(closed)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				close(connected)
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				io.Copy(io.Discard, conn) // Never answer the SOCKS greeting.
			}()
			store, err := New(config.Config{Provider: provider, Region: "us-east-1", Endpoint: "https://storage.example", Bucket: "books", AccessKeyID: "fixture-id", SecretAccessKey: base64.StdEncoding.EncodeToString(make([]byte, 64)), SOCKS5Proxy: "socks5://" + listener.Addr().String()})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := store.Get(ctx, "book.azw3", 0, ""); done <- err }()
			select {
			case <-connected:
			case <-time.After(2 * time.Second):
				t.Fatal("proxy not reached")
			}
			cancel()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("cancelled request succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("cancellation stalled")
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("abandoned proxy handshake leaked")
			}
		})
	}
}
