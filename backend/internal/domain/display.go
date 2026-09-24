package domain

import (
	"html"
	"regexp"
	"strings"
	"unicode"
)

var (
	opaqueUUID    = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	emTagRegex    = regexp.MustCompile(`(?i)\[em\]e\d+\[/em\]`)
	qzoneAtRegex  = regexp.MustCompile(`(?i)@\{uin:\d+,\s*nick:([^,}]+)[^}]*\}`)
	cqCodeRegex   = regexp.MustCompile(`(?i)\[CQ:[^\]]+\]`)
	multiSpaceReg = regexp.MustCompile(`[ \t\f\v]+`)
)

// CleanDisplayText removes transport control bytes, escape artifacts, and QQ rich-text prefixes
// from names and texts without changing the raw value retained by the collectors.
func CleanDisplayText(value string) string {
	if value == "" {
		return ""
	}

	// HTML unescape (&amp;, &lt;, &gt;, &quot;, &#39;, &nbsp;, etc.)
	value = html.UnescapeString(value)

	// Literal escapes: \t, \r, \n
	value = strings.ReplaceAll(value, `\t`, " ")
	value = strings.ReplaceAll(value, `\r`, "")
	value = strings.ReplaceAll(value, `\n`, "\n")

	// QZone @ mentions & emotions
	value = qzoneAtRegex.ReplaceAllString(value, "@$1 ")
	value = emTagRegex.ReplaceAllString(value, "[表情]")

	// Remove transport control characters (excluding newline, tab & ZWJ)
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == '\u200d' {
			return r
		}
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, value)

	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "<$") {
		if end := strings.IndexByte(value, '>'); end >= 0 && end <= 32 {
			value = strings.TrimSpace(value[end+1:])
		}
	}

	// Normalize spaces
	value = multiSpaceReg.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func IsOpaqueIdentifier(value string) bool {
	return opaqueUUID.MatchString(strings.TrimSpace(value))
}
