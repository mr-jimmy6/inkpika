package i18n

import (
	"embed"
	"encoding/json"
	"html"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

//go:embed zh.json runtime.js
var resources embed.FS

var chinese map[string]string
var english map[string]string

type rule struct {
	expression *regexp.Regexp
	parts      []string
	nested     []bool
}

var toChinese, toEnglish []rule
var verbs = regexp.MustCompile(`%[dsqw]`)

func init() {
	b, _ := resources.ReadFile("zh.json")
	if err := json.Unmarshal(b, &chinese); err != nil {
		panic(err)
	}
	english = make(map[string]string, len(chinese))
	keys := make([]string, 0, len(chinese))
	for en, zh := range chinese {
		english[zh] = en
		keys = append(keys, en)
	}
	// Prefer more specific templates over short prefixes.
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) == len(keys[j]) {
			return keys[i] < keys[j]
		}
		return len(keys[i]) > len(keys[j])
	})
	for _, en := range keys {
		zh := chinese[en]
		if verbs.MatchString(en) {
			toChinese = append(toChinese, compile(en, zh))
			toEnglish = append(toEnglish, compile(zh, en))
		} else if strings.HasSuffix(en, ": ") {
			toChinese = append(toChinese, compile(en+"%w", zh+"%w"))
			toEnglish = append(toEnglish, compile(zh+"%w", en+"%w"))
		}
	}
}

func compile(source, target string) rule {
	positions := verbs.FindAllStringIndex(source, -1)
	pattern, last := "(?s)^", 0
	var nested []bool
	for _, pos := range positions {
		pattern += regexp.QuoteMeta(source[last:pos[0]])
		switch source[pos[0]:pos[1]] {
		case "%d":
			pattern += `(-?[0-9]+)`
		case "%q":
			pattern += `("(?:[^"\\]|\\.)*")`
		default:
			pattern += `(.*?)`
		}
		nested = append(nested, source[pos[0]:pos[1]] == "%w")
		last = pos[1]
	}
	pattern += regexp.QuoteMeta(source[last:]) + "$"
	return rule{regexp.MustCompile(pattern), verbs.Split(target, -1), nested}
}

// Text translates only application messages. Callers must not pass filenames,
// object keys, credentials or arbitrary user data. Unknown messages stay intact.
// Reverse templates support human messages in pre-localization saved state.
func Text(language, message string) string { return translate(language, message, 0) }
func translate(language, message string, depth int) string {
	if depth > 8 {
		return message
	}
	catalog, rules := english, toEnglish
	if language == "zh" {
		catalog, rules = chinese, toChinese
	}
	if value, ok := catalog[message]; ok {
		return value
	}
	for _, r := range rules {
		m := r.expression.FindStringSubmatch(message)
		if m == nil || len(r.parts) != len(m) {
			continue
		}
		out := r.parts[0]
		for i, arg := range m[1:] {
			if r.nested[i] {
				arg = translate(language, arg, depth+1)
			}
			out += arg + r.parts[i+1]
		}
		return out
	}
	return message
}

// JSON translates an explicit allowlist of human-facing fields, never data
// such as names, paths, tokens or machine-readable status/provider values.
func JSON(w http.ResponseWriter, code int, value interface{}) {
	b, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "Cannot encode response", 500)
		return
	}
	var data interface{}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.UseNumber()
	if err = d.Decode(&data); err != nil {
		http.Error(w, "Cannot encode response", 500)
		return
	}
	localizeFields(w.Header().Get("Content-Language"), data)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func localizeFields(language string, value interface{}) {
	switch v := value.(type) {
	case []interface{}:
		for _, item := range v {
			localizeFields(language, item)
		}
	case map[string]interface{}:
		for key, item := range v {
			switch key {
			case "error", "reason", "message", "detail", "network", "access", "read", "provider_label", "connection", "title", "description", "stop_label":
				if message, ok := item.(string); ok {
					v[key] = Text(language, message)
				}
			default:
				localizeFields(language, item)
			}
		}
	}
}

// Script is shared by the device and static preview. The server supplies its
// language explicitly; only static Pages use an explicit ?lang= switch.
func Script(language string, extra ...map[string]string) []byte {
	merged := make(map[string]string, len(chinese))
	for key, value := range chinese {
		merged[key] = value
	}
	for _, catalog := range extra {
		for key, value := range catalog {
			merged[key] = value
		}
	}
	b, _ := json.Marshal(merged)
	runtime, _ := resources.ReadFile("runtime.js")
	prefix := ""
	if language != "" {
		v, _ := json.Marshal(language)
		prefix = "window.InkPikaLocale = " + string(v) + ";\n"
	}
	return []byte(prefix + "window.InkPikaMessages = " + string(b) + ";\n" + string(runtime))
}

func ServeScript(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write(Script(w.Header().Get("Content-Language")))
}

var htmlText = regexp.MustCompile(`>([^<]+)<`)
var htmlAttribute = regexp.MustCompile(`(title|alt|placeholder|aria-label)="([^"]*)"`)

// HTML translates only embedded static assets, never an API response or user
// markup. It also provides correct initial text before the old WebKit runs JS.
func HTML(data []byte, language string) []byte {
	translateStatic := func(value string) string {
		value = html.UnescapeString(value)
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return value
		}
		return strings.Replace(value, trimmed, Text(language, trimmed), 1)
	}
	s := htmlText.ReplaceAllStringFunc(string(data), func(part string) string {
		return ">" + html.EscapeString(translateStatic(part[1:len(part)-1])) + "<"
	})
	s = htmlAttribute.ReplaceAllStringFunc(s, func(part string) string {
		m := htmlAttribute.FindStringSubmatch(part)
		return m[1] + `="` + html.EscapeString(translateStatic(m[2])) + `"`
	})
	if language == "zh" {
		s = strings.Replace(s, `lang="en"`, `lang="zh-CN"`, 1)
	}
	return []byte(s)
}
