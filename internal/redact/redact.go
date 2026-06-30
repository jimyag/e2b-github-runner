package redact

import (
	"net/url"
	"regexp"
	"strings"
)

var passwordFieldPattern = regexp.MustCompile(`(?i)(password\s*=\s*)(?:'[^']*'|"[^"]*"|[^\s]+)`)

// DatabaseDSN removes credentials from database DSNs while preserving location fields.
func DatabaseDSN(raw string) string {
	u, err := url.Parse(raw)
	if err == nil && u.Scheme != "" && u.User != nil {
		u.User = url.User("xxxxx")
		return u.String()
	}

	at := strings.Index(raw, "@")
	colon := strings.Index(raw, ":")
	if at > 0 && colon > 0 && colon < at {
		return "xxxxx" + raw[at:]
	}
	return passwordFieldPattern.ReplaceAllString(raw, `${1}xxxxx`)
}
