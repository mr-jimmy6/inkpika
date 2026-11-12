package syncer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestMenuContainsAllStatusesAndFullDetails(t *testing.T) {
	s := Snapshot{Known: true, Fresh: true, Total: 14, Downloaded: 9, Pending: 5, Phase: "idle"}
	states := []string{"downloaded", "pending", "update", "failed", "downloading"}
	for i := 0; i < 14; i++ {
		s.Books = append(s.Books, Item{Key: fmt.Sprintf("sync/目录/电子书-%02d.azw3", i), Status: states[i%len(states)], Size: 100, Modified: int64(200 - i)})
	}
	longName := "sync/目录/" + strings.Repeat("很长的书名", 20) + ".azw3"
	s.Books[13].Key = longName
	file := filepath.Join(t.TempDir(), "menu.json")
	if err := WriteMenu(file, s, MenuOptions{}); err != nil {
		t.Fatal(err)
	}
	var menu struct{ Items []menuItem }
	b, _ := os.ReadFile(file)
	if err := json.Unmarshal(b, &menu); err != nil {
		t.Fatal(err)
	}
	top := menu.Items[0].Items
	if len(top) != 7 || top[1].Name != "Start sync" || top[2].Items[0].Params != "refresh" || top[3].Action != "./start.sh" || top[4].Action != "./transfer-start.sh" || top[6].Items[0].Action != "./service.sh" {
		t.Fatalf("unexpected top-level actions: %+v", top)
	}
	books := top[0].Items
	if len(books) != 14 {
		t.Fatal("book list truncated", len(books))
	}
	for i, code := range []string{"[OK] ", "[NEW] ", "[UPD] ", "[ERR] ", "[DL] "} {
		if !strings.HasPrefix(books[i].Name, code) {
			t.Fatal(books[i].Name)
		}
	}
	full := ""
	for _, line := range books[13].Items {
		if strings.HasPrefix(line.Name, "Status: ") {
			break
		}
		full += line.Name
	}
	if full != "File: "+strings.TrimPrefix(longName, Prefix) {
		t.Fatal("detail lost filename characters", full)
	}
	s.Running = true
	if err := WriteMenu(file, s, MenuOptions{}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(file)
	json.Unmarshal(b, &menu)
	if menu.Items[0].Items[1].Params != "cancel" {
		t.Fatal("stop not accessible from main menu")
	}
}

func TestServiceChoicesAreConditionalAndRefreshIsNested(t *testing.T) {
	file := filepath.Join(t.TempDir(), "menu.json")
	if err := WriteMenu(file, Snapshot{Phase: "idle"}, MenuOptions{ServicePID: 123, TransferPID: 456, SettingsPID: 789}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(file)
	if strings.Contains(string(b), "secret-") {
		t.Fatal("credentials leaked")
	}
	var menu struct{ Items []menuItem }
	json.Unmarshal(b, &menu)
	top := menu.Items[0].Items
	for _, item := range top {
		if item.Name == "Refresh status" {
			t.Fatal("refresh is still top-level")
		}
	}
	if top[2].Items[0].Params != "refresh" {
		t.Fatal("refresh missing from status")
	}
	service := top[6]
	if len(service.Items) != 6 || service.Items[0].If != service.Items[1].If+" !" || !strings.Contains(service.Items[1].If, "/proc/123/cmdline") {
		t.Fatal("service actions are not mutually exclusive", service)
	}
	if service.Items[2].If != service.Items[3].If+" !" || !strings.Contains(service.Items[3].If, "/proc/456/cmdline") {
		t.Fatal("transfer service state is not independent", service)
	}
	// PW2's KUAL reads only argv[0] from the NUL-separated cmdline. Requiring
	// a mode argument here incorrectly shows "start" for a running service.
	for _, item := range []menuItem{service.Items[1], service.Items[3]} {
		fields := strings.Fields(item.If)
		pattern, err := strconv.Unquote(fields[1])
		if err != nil {
			t.Fatal(err)
		}
		re := regexp.MustCompile(pattern)
		if !re.MatchString("/mnt/us/extensions/inkpika/bin/inkpika") || re.MatchString("/mnt/us/extensions/inkpika/bin/inkpika-other") || re.MatchString("/bin/sh") {
			t.Fatal("predicate must match only this executable's first record", item.If)
		}
	}
	if top[3].Name != "Cloud library [web]" {
		t.Fatal(top[3])
	}
}

func TestLegacySnapshotRefreshBuildsCompleteCatalogue(t *testing.T) {
	e, store, m, c := setup(t)
	for i := 0; i < 9; i++ {
		put(store, fmt.Sprintf("sync/%d.txt", i), "book", "v1", int64(i+1))
	}
	run(t, e, true)
	e.Close()
	b, err := os.ReadFile(filepath.Join(c.StateDir, "sync.json"))
	if err != nil {
		t.Fatal(err)
	}
	var legacy diskState
	json.Unmarshal(b, &legacy)
	legacy.Snapshot.Books = nil
	b, _ = json.Marshal(legacy)
	os.WriteFile(filepath.Join(c.StateDir, "sync.json"), b, 0600)
	newer, err := New(c, store, m, "")
	if err != nil {
		t.Fatal(err)
	}
	defer newer.Close()
	if newer.Snapshot().Books != nil || newer.Snapshot().Fresh {
		t.Fatal("legacy catalogue fabricated")
	}
	if err = newer.Refresh(); err != nil {
		t.Fatal(err)
	}
	after := wait(t, newer)
	if len(after.Books) != 9 || after.Downloaded != 9 || after.Fetched != 0 {
		t.Fatal(after)
	}
}

func TestMenuLanguagePreservesBookKeysAndActions(t *testing.T) {
	s := Snapshot{Known: true, Fresh: true, Phase: "idle", Total: 1, Pending: 1, Books: []Item{{Key: "sync/Ready.azw3", Status: "pending", Reason: "Not downloaded"}}}
	for _, language := range []string{"en", "zh"} {
		file := filepath.Join(t.TempDir(), "menu.json")
		if err := WriteMenu(file, s, MenuOptions{Language: language}); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		expected := "Start sync"
		if language == "zh" {
			expected = "开始同步"
		}
		if !strings.Contains(string(b), expected) || !strings.Contains(string(b), "Ready.azw3") || !strings.Contains(string(b), `"params": "start"`) {
			t.Fatal(string(b))
		}
	}
}

func TestSubmenuNavigationDoesNotRefreshKUAL(t *testing.T) {
	assertNavigation := func(t *testing.T, filename string) {
		t.Helper()
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		var menu struct{ Items []menuItem }
		if err := json.Unmarshal(data, &menu); err != nil {
			t.Fatal(err)
		}
		var visit func([]menuItem)
		visit = func(items []menuItem) {
			for _, item := range items {
				if len(item.Items) > 0 {
					if item.Refresh || item.Exit {
						t.Fatalf("entering submenu %q must not reset or exit KUAL", item.Name)
					}
					visit(item.Items)
				} else if item.Action == "./sync.sh" || strings.HasSuffix(item.Action, "service.sh") || strings.HasSuffix(item.Action, "stop.sh") {
					if !item.Refresh {
						t.Fatalf("state-changing action %q must still refresh", item.Name)
					}
				}
			}
		}
		visit(menu.Items)
	}
	t.Run("bootstrap", func(t *testing.T) {
		assertNavigation(t, "../../extensions/inkpika/menu.json")
	})
	for _, language := range []string{"en", "zh"} {
		t.Run(language, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "menu.json")
			snapshot := Snapshot{Phase: "idle", Known: true, Total: 1, Books: []Item{{Key: "sync/example.txt", Status: "pending"}}}
			if err := WriteMenu(filename, snapshot, MenuOptions{Language: language, ServicePID: 123, TransferPID: 456, SettingsPID: 789}); err != nil {
				t.Fatal(err)
			}
			assertNavigation(t, filename)
		})
	}
}
