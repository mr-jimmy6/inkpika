package transfer

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// currentEssid is read-only; profileData may contain saved network credentials
// and must never be queried or sent to a browser.
func WiFiName() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "lipc-hash-prop", "-n", "com.lab126.wifid", "currentEssid").Output()
	if err != nil {
		return ""
	}
	return parseWiFiName(string(b))
}

var essidLine = regexp.MustCompile(`(?m)^\s*essid = (".*")\s*$`)

func parseWiFiName(value string) string {
	if !strings.Contains(value, `connected = "yes"`) {
		return ""
	}
	m := essidLine.FindStringSubmatch(value)
	if len(m) != 2 {
		return ""
	}
	name, err := strconv.Unquote(strings.TrimSpace(m[1]))
	if err != nil {
		return ""
	}
	return name
}
