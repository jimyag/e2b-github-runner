package redact

import (
	"net/url"
	"strings"
)

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
	return raw
}
