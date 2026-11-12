package syncer

import (
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mr-jimmy6/inkpika/internal/i18n"
	"github.com/mr-jimmy6/inkpika/internal/statefile"
)

type menuItem struct {
	Name     string     `json:"name"`
	Action   string     `json:"action,omitempty"`
	Params   string     `json:"params,omitempty"`
	Items    []menuItem `json:"items,omitempty"`
	Exit     bool       `json:"exitmenu"`
	Refresh  bool       `json:"refresh,omitempty"`
	Status   bool       `json:"status"`
	Internal string     `json:"internal,omitempty"`
	If       string     `json:"if,omitempty"`
}

type MenuOptions struct {
	Language    string
	ServicePID  int
	TransferPID int
	SettingsPID int
}

func cleanLabel(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
func label(s string, max int) string {
	s = cleanLabel(s)
	width := 0
	for i, r := range s {
		step := 1
		if r > 127 {
			step = 2
		}
		if width+step > max-1 {
			return s[:i] + "…"
		}
		width += step
	}
	return s
}
func info(s string) menuItem { return menuItem{Name: label(s, 38), Action: ":"} }

// Details retain every character, across native KUAL rows/pages if necessary.
func infoLines(s string) []menuItem {
	s = cleanLabel(s)
	var out []menuItem
	start, width := 0, 0
	for i, r := range s {
		step := 1
		if r > 127 {
			step = 2
		}
		if width+step > 36 {
			out = append(out, info(s[start:i]))
			start, width = i, 0
		}
		width += step
	}
	return append(out, info(s[start:]))
}

// Internal messages must be ASCII: KUAL awk counts bytes, Java counts characters.
func control(name, command string) menuItem {
	return menuItem{Name: name, Action: "./sync.sh", Params: command, Refresh: true, Internal: "status Submitted. Refresh status for results."}
}
func date(t int64) string {
	if t <= 0 {
		return "Unknown time"
	}
	return time.Unix(t, 0).Local().Format("01-02 15:04")
}
func statusCode(status string) string {
	switch status {
	case "downloaded":
		return "OK"
	case "update":
		return "UPD"
	case "failed":
		return "ERR"
	case "downloading":
		return "DL"
	default:
		return "NEW"
	}
}
func statusLabel(status string) string {
	switch status {
	case "downloaded":
		return "Downloaded; matches remote"
	case "update":
		return "Update or verification pending"
	case "failed":
		return "Check or download failed"
	case "downloading":
		return "Downloading"
	default:
		return "Not downloaded"
	}
}

// Only fixed commands/parameters are executable. Object keys remain plain text.
func WriteMenu(filename string, s Snapshot, opt MenuOptions) error {
	language := i18n.Resolve(opt.Language)
	t := func(message string) string { return i18n.Text(language, message) }
	summary := t("Not checked yet")
	if s.Known {
		summary = fmt.Sprintf(t("%d/%d downloaded"), s.Downloaded, s.Total)
	}
	if s.Known && !s.Fresh {
		summary = t("Cached ") + summary
	}
	phase := map[string]string{"idle": t("Ready"), "scanning": t("Checking"), "downloading": t("Syncing"), "cancelled": t("Stopped"), "error": t("Partially completed")}[s.Phase]
	details := []menuItem{control(t("Refresh status"), "refresh"), info(t("Status: ") + phase), info(fmt.Sprintf(t("Total %d Saved %d Pending %d"), s.Total, s.Downloaded, s.Pending)), info(t("Checked: ") + t(date(s.Updated))), info(fmt.Sprintf(t("Downloaded %d · Skipped %d"), s.Fetched, s.Skipped))}
	if !s.Fresh {
		details = append(details, info(t("Cached results; refresh to verify")))
	}
	if s.Current != "" {
		details = append(details, infoLines(t("Current: ")+s.Current)...)
	}
	if s.Error != "" {
		details = append(details, infoLines(t(s.Error))...)
	}
	details = append(details, menuItem{Name: t("Status codes"), Items: []menuItem{info(t("OK = Downloaded; version matches")), info(t("NEW = Not downloaded")), info(t("UPD = Update or verification pending")), info(t("ERR = Check or download failed")), info(t("DL = Downloading"))}})
	books := make([]menuItem, 0, len(s.Books))
	for _, book := range s.Books {
		d := infoLines(t("File: ") + strings.TrimPrefix(book.Key, Prefix))
		d = append(d, info(t("Status: ")+t(statusLabel(book.Status))), info(t("Updated: ")+t(date(book.Modified))), info(fmt.Sprintf(t("%s · %d bytes"), strings.ToUpper(strings.TrimPrefix(path.Ext(book.Key), ".")), book.Size)))
		if book.Reason != "" {
			d = append(d, infoLines(t(book.Reason))...)
		}
		books = append(books, menuItem{Name: "[" + statusCode(book.Status) + "] " + label(path.Base(book.Key), 30), Items: d})
	}
	if len(books) == 0 {
		message := t("No ebooks to sync")
		if s.Books == nil || !s.Known {
			message = t("Refresh status to load all books")
		}
		books = append(books, info(message))
	}
	start := control(t("Start sync"), "start")
	if s.Running {
		start = control(t("Stop sync"), "cancel")
	}
	stateLabel := t("View status")
	if s.Running {
		stateLabel += " · " + phase
	} else if s.Pending > 0 {
		stateLabel += fmt.Sprintf(t(" · Pending %d"), s.Pending)
	}
	service := []menuItem{{Name: t("Start cloud library"), Action: "./service.sh", Refresh: true}}
	if opt.ServicePID > 0 {
		// Native KUAL reevaluates this predicate on entering the submenu. Match
		// argv[0] as well as PID. PW2's awk getline stops at the first NUL,
		// so this predicate must not require command arguments. RefreshMenu
		// identifies each service from its own PID file and full cmdline.
		predicate := strconv.Quote(fmt.Sprintf("/proc/%d/cmdline", opt.ServicePID)) + ` "^/mnt/us/extensions/inkpika/bin/inkpika([[:cntrl:]]|$)" -gg`
		service[0].If = predicate + " !"
		service = append(service, menuItem{Name: t("Stop cloud library"), Action: "./stop.sh", Refresh: true, If: predicate})
	}
	transferStart := menuItem{Name: t("Start wireless transfer"), Action: "./transfer-service.sh", Refresh: true}
	if opt.TransferPID > 0 {
		predicate := strconv.Quote(fmt.Sprintf("/proc/%d/cmdline", opt.TransferPID)) + ` "^/mnt/us/extensions/inkpika/bin/inkpika([[:cntrl:]]|$)" -gg`
		transferStart.If = predicate + " !"
		service = append(service, transferStart, menuItem{Name: t("Stop wireless transfer"), Action: "./transfer-stop.sh", Refresh: true, If: predicate})
	} else {
		service = append(service, transferStart)
	}
	settingsStart := menuItem{Name: t("Start settings"), Action: "./settings-service.sh", Refresh: true}
	if opt.SettingsPID > 0 {
		predicate := strconv.Quote(fmt.Sprintf("/proc/%d/cmdline", opt.SettingsPID)) + ` "^/mnt/us/extensions/inkpika/bin/inkpika([[:cntrl:]]|$)" -gg`
		settingsStart.If = predicate + " !"
		service = append(service, settingsStart, menuItem{Name: t("Stop settings"), Action: "./settings-stop.sh", Refresh: true, If: predicate})
	} else {
		service = append(service, settingsStart)
	}
	// KUAL reloads the entire menu for refresh=true and resets navigation.
	// Only actions request refresh; entering a submenu must keep its position.
	root := struct {
		Items []menuItem `json:"items"`
	}{[]menuItem{{Name: "InkPika", Items: []menuItem{
		{Name: t("All books · ") + summary, Items: books}, start,
		{Name: stateLabel, Items: details},
		{Name: t("Cloud library [web]"), Action: "./start.sh", Exit: true},
		{Name: t("Wireless transfer [web]"), Action: "./transfer-start.sh", Exit: true},
		{Name: t("Settings [web]"), Action: "./settings-start.sh", Exit: true},
		{Name: t("Background services"), Items: service},
	}}}}
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return statefile.Write(filename, b)
}
