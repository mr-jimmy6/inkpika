// Preview exercises the shared UI and downloader using synthetic in-memory books.
// Provider protocols are tested separately against local HTTP/TLS fixtures.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/config"
	"github.com/mr-jimmy6/inkpika/internal/download"
	"github.com/mr-jimmy6/inkpika/internal/storage"
	"github.com/mr-jimmy6/inkpika/internal/web"
)

type fixtureStore struct{ books map[string]string }

func (s *fixtureStore) List(ctx context.Context, prefix, marker string) (storage.Listing, error) {
	out := storage.Listing{Keys: []storage.Object{}, Folders: []string{}}
	seen := map[string]bool{}
	var names []string
	for name := range s.books {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		tail := strings.TrimPrefix(name, prefix)
		if i := strings.Index(tail, "/"); i >= 0 {
			folder := prefix + tail[:i+1]
			if !seen[folder] {
				out.Folders = append(out.Folders, folder)
				seen[folder] = true
			}
		} else {
			out.Keys = append(out.Keys, storage.Object{Key: name, Size: int64(len(s.books[name])), Modified: 1793979982})
		}
	}
	if prefix == "" {
		if marker == "" {
			out.Folders = []string{"文学/", "技术/"}
			out.HasMore = true
			out.NextMarker = "page2"
		} else {
			out.Folders = []string{"第二页/"}
			out.Keys = []storage.Object{}
		}
	}
	return out, nil
}

func (s *fixtureStore) Head(ctx context.Context, key string) (*http.Response, error) {
	data, ok := s.books[key]
	if !ok {
		return nil, fmt.Errorf("Sample file missing")
	}
	return &http.Response{StatusCode: 200, ContentLength: int64(len(data)), Header: http.Header{"Etag": {`"fixture-v1"`}}, Body: http.NoBody}, nil
}

func (s *fixtureStore) Get(ctx context.Context, key string, offset int64, etag string) (*http.Response, error) {
	data, ok := s.books[key]
	if !ok || offset < 0 || offset > int64(len(data)) {
		return nil, fmt.Errorf("Sample file missing or invalid resume range")
	}
	header := http.Header{"Etag": {`"fixture-v1"`}}
	code := 200
	if offset > 0 {
		code = 206
		header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, len(data)-1, len(data)))
	}
	data = data[offset:]
	length := int64(len(data))
	reader, writer := io.Pipe()
	go func() { <-ctx.Done(); reader.CloseWithError(ctx.Err()) }()
	go func() {
		defer writer.Close()
		for len(data) > 0 {
			n := 4096
			if n > len(data) {
				n = len(data)
			}
			if _, err := writer.Write([]byte(data[:n])); err != nil {
				return
			}
			data = data[n:]
			select {
			case <-ctx.Done():
				return
			case <-time.After(80 * time.Millisecond):
			}
		}
	}()
	return &http.Response{StatusCode: code, ContentLength: length, Header: header, Body: reader}, nil
}

func main() {
	provider := flag.String("provider", config.AliyunOSS, "aliyun_oss, qingstor or s3 (display only; no cloud requests)")
	language := flag.String("language", "en", "Interface language: en, zh or auto")
	flag.Parse()
	if *provider != config.AliyunOSS && *provider != config.QingStor && *provider != config.S3 {
		log.Fatal("unsupported demo provider")
	}
	content := strings.Repeat("这是本地界面测试数据，不是真实电子书。\n", 2048)
	store := &fixtureStore{books: map[string]string{"人类群星闪耀时.azw3": content, "瓦尔登湖.mobi": content, "阅读清单.txt": "这是下载功能测试文件。\n", "待转换的书.epub": content, "文学/城南旧事.azw3": content, "技术/Go 语言笔记.pdf": content, "第二页/中文 + & # = %.txt": "特殊文件名测试\n"}}
	root, err := filepath.Abs("local/preview")
	if err != nil {
		log.Fatal(err)
	}
	c := config.Config{Language: *language, Provider: *provider, Bucket: "demo-library", Region: "Local preview", Endpoint: "https://demo.invalid", Listen: "127.0.0.1:8788", DownloadDir: filepath.Join(root, "documents"), StateDir: filepath.Join(root, fmt.Sprintf("state-%d", os.Getpid())), MaxDownloadMB: 512}
	manager, err := download.New(c, store)
	if err != nil {
		log.Fatal(err)
	}
	defer manager.Close()
	token := strings.Repeat("a", 64)
	launch := "http://" + c.Listen + "/#token=" + token
	if err = os.WriteFile(filepath.Join(c.StateDir, "launch.url"), []byte(launch), 0600); err != nil {
		log.Fatal(err)
	}
	fmt.Println("LOCAL DEMO ONLY (synthetic books): " + launch)
	log.Fatal(http.ListenAndServe(c.Listen, web.New(c, store, manager, token)))
}
