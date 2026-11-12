// Generate the public demo from production assets and public book fixtures.
// Run from the repository root: GOTOOLCHAIN=go1.23.12 go run ./scripts/site-demo
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mr-jimmy6/inkpika/internal/buildinfo"
	"github.com/mr-jimmy6/inkpika/internal/i18n"
	"github.com/mr-jimmy6/inkpika/internal/syncer"
	qrcode "github.com/skip2/go-qrcode"
)

type item struct {
	Name    string `json:"name"`
	Items   []item `json:"items,omitempty"`
	Action  string `json:"action,omitempty"`
	Params  string `json:"params,omitempty"`
	Preview string `json:"preview,omitempty"`
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func read(name string) []byte { b, err := os.ReadFile(name); must(err); return b }
func write(name string, b []byte) {
	must(os.MkdirAll(filepath.Dir(name), 0755))
	must(os.WriteFile(name, b, 0644))
}

// Shell actions, process predicates and session data must never reach the demo.
func sanitize(items []item) []item {
	actions := map[string]string{
		"./start.sh": "open-cloud", "./transfer-start.sh": "open-wireless", "./settings-start.sh": "open-settings",
		"./service.sh": "start-cloud", "./stop.sh": "stop-cloud",
		"./transfer-service.sh": "start-wireless", "./transfer-stop.sh": "stop-wireless",
		"./settings-service.sh": "start-settings", "./settings-stop.sh": "stop-settings",
	}
	for i := range items {
		it := &items[i]
		if it.Action == "./sync.sh" {
			if it.Params != "start" && it.Params != "cancel" && it.Params != "refresh" {
				panic("unmapped sync action: " + it.Params)
			}
			it.Preview = "sync-" + it.Params
		} else if it.Action != "" && it.Action != ":" {
			var ok bool
			it.Preview, ok = actions[it.Action]
			if !ok {
				panic("unmapped menu action: " + it.Action)
			}
		}
		it.Action, it.Params = "", ""
		it.Items = sanitize(it.Items)
	}
	return items
}

func main() {
	// Keep fixture menu timestamps independent of the developer's time zone.
	time.Local = time.FixedZone("Asia/Shanghai", 8*60*60)
	var pageCatalog map[string]string
	must(json.Unmarshal(read("site/locales/zh.json"), &pageCatalog))
	write("site/assets/i18n.js", i18n.Script("", pageCatalog))
	const dest = "site/preview/native/"
	for _, entry := range []struct{ name, source, html, js, css string }{
		{"cloud", "internal/web/assets/", "index.html", "app.js", "internal/web/assets/style.css"},
		{"wireless", "internal/transfer/assets/", "upload.html", "upload.js", "internal/transfer/assets/style.css"},
		{"settings", "internal/settings/assets/", "upload.html", "upload.js", "internal/transfer/assets/style.css"},
		{"wireless-device", "internal/transfer/assets/", "device.html", "device.js", "internal/transfer/assets/style.css"},
		{"settings-device", "internal/settings/assets/", "device.html", "device.js", "internal/transfer/assets/style.css"},
	} {
		html := string(buildinfo.HTML(read(entry.source + entry.html)))
		html = strings.ReplaceAll(html, `href="/device.css"`, `href="settings-detail.css"`)
		html = strings.ReplaceAll(html, `src="/connection.js"`, `src="wireless-device.js"`)
		html = strings.ReplaceAll(html, `src="/i18n.js"`, `src="../../assets/i18n.js"`)
		html = strings.ReplaceAll(html, `href="/style.css"`, `href="`+entry.name+`.css"`)
		html = strings.ReplaceAll(html, `<body>`, `<body data-demo-screen="`+entry.name+`">`)
		html = strings.ReplaceAll(html, `<script src="/app.js"></script>`, `<script src="../bridge.js"></script><script src="`+entry.name+`.js"></script>`)
		html = strings.ReplaceAll(html, "</head>", `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; connect-src 'none'; form-action 'none'; base-uri 'none'"><link rel="icon" href="../../assets/favicon.svg"><link rel="stylesheet" href="../native-fit.css"></head>`)
		write(dest+entry.name+".html", []byte(html))
		write(dest+entry.name+".js", read(entry.source+entry.js))
		write(dest+entry.name+".css", read(entry.css))
	}
	write(dest+"settings-detail.css", read("internal/settings/assets/device.css"))
	var sample struct {
		Records map[string]struct {
			Snapshot syncer.Snapshot `json:"snapshot"`
		} `json:"records"`
	}
	must(json.Unmarshal(read("site/assets/previews/sync-sample.json"), &sample))
	tmp, err := os.MkdirTemp("", "inkpika-demo-menu-")
	must(err)
	defer os.RemoveAll(tmp)
	menuLocales := map[string]map[string][]item{}
	for _, language := range []string{"en", "zh"} {
		menus := map[string][]item{}
		for _, phase := range []string{"before", "after", "running", "cancelled", "scanning", "error"} {
			s := sample.Records["before"].Snapshot
			s.Updated = 1794096000
			switch phase {
			case "after":
				s = sample.Records["after"].Snapshot
				s.Updated = 1794096000
			case "running":
				s.Phase = "downloading"
				s.Running = true
			case "scanning":
				s.Phase = "scanning"
				s.Running = true
			case "cancelled":
				s.Phase = "cancelled"
			case "error":
				s.Phase = "error"
				s.Error = "Sample: connection interrupted; saved books preserved."
			}
			path := filepath.Join(tmp, "menu.json")
			must(syncer.WriteMenu(path, s, syncer.MenuOptions{Language: language, ServicePID: 1, TransferPID: 2, SettingsPID: 3}))
			var menu struct {
				Items []item `json:"items"`
			}
			must(json.Unmarshal(read(path), &menu))
			menus[phase] = sanitize(menu.Items[0].Items)
		}
		menuLocales[language] = menus
	}
	books := []map[string]interface{}{}
	for _, name := range []string{"01_中文排版测试.azw3", "02_English + 空格测试.azw3", "03_目录与列表测试.azw3"} {
		data := read(filepath.Join("testdata/books", name))
		books = append(books, map[string]interface{}{"name": name, "size": len(data), "base64": base64.StdEncoding.EncodeToString(data)})
	}
	qr, err := qrcode.Encode("InkPika - local sample preview only", qrcode.Medium, 300)
	must(err)
	b, err := json.MarshalIndent(map[string]interface{}{"menu_locales": menuLocales, "books": books, "qr": "data:image/png;base64," + base64.StdEncoding.EncodeToString(qr)}, "", "  ")
	must(err)
	write("site/preview/sample-data.js", append(append([]byte("/* Generated by scripts/site-demo; public fixtures only. */\nwindow.inkpikaDemoData = "), b...), []byte(";\n")...))
	fmt.Println("Generated five native pages, full KUAL menus and three public sample books.")
}
