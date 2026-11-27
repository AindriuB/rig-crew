// Package frontmatter reads and writes flat "key: value" front matter.
package frontmatter

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Field is one front matter entry; order is kept.
type Field struct {
	Key   string
	Value string
	// Quoted is set by Parse when the value was double-quoted in the source.
	// Format quotes values YAML reads as non-string only when Quoted is true.
	Quoted bool
}

// Parse splits data into front matter fields and body. CRLF input is accepted
// and the result uses LF.
func Parse(data []byte) ([]Field, string, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return nil, "", errors.New("front matter: file does not start with ---")
	}
	var fields []Field
	for {
		if rest == "" {
			return nil, "", errors.New("front matter: no closing ---")
		}
		line, after, _ := strings.Cut(rest, "\n")
		rest = after
		if line == "---" {
			return fields, rest, nil
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, val, found := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, "", fmt.Errorf("front matter: line %q is not key: value", line)
		}
		val = strings.TrimSpace(val)
		quoted := strings.HasPrefix(val, `"`)
		if quoted {
			u, err := strconv.Unquote(val)
			if err != nil {
				return nil, "", fmt.Errorf("front matter: bad quoted value for %q: %w", key, err)
			}
			val = u
		}
		fields = append(fields, Field{Key: key, Value: val, Quoted: quoted})
	}
}

// Format writes fields and body as a front matter document.
func Format(fields []Field, body string) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	for _, f := range fields {
		b.WriteString(f.Key + ": " + quoteIfNeeded(f.Value, f.Quoted) + "\n")
	}
	b.WriteString("---\n")
	b.WriteString(body)
	return []byte(b.String())
}

func quoteIfNeeded(v string, quoted bool) string {
	if needsQuote(v, quoted) {
		return strconv.Quote(v)
	}
	return v
}

func needsQuote(v string, quoted bool) bool {
	if v == "" || v != strings.TrimSpace(v) {
		return true
	}
	if strings.Contains(v, ": ") || strings.Contains(v, " #") || strings.HasSuffix(v, ":") {
		return true
	}
	if strings.ContainsAny(v, "\n\r\t") {
		return true
	}
	if quoted && nonString(v) {
		return true
	}
	return strings.ContainsRune("-?:,[]{}#&*!|>'\"%@`", rune(v[0]))
}

// nonString reports whether YAML 1.1 or 1.2 would read v as a bool, null or
// number rather than a string.
func nonString(v string) bool {
	l := strings.ToLower(v)
	switch l {
	case "true", "false", "yes", "no", "on", "off", "y", "n", "null", "~",
		".inf", "+.inf", "-.inf", ".nan":
		return true
	}
	n := strings.ReplaceAll(l, "_", "")
	if _, err := strconv.ParseInt(n, 0, 64); err == nil {
		return true
	}
	if _, err := strconv.ParseUint(n, 0, 64); err == nil {
		return true
	}
	_, err := strconv.ParseFloat(n, 64)
	return err == nil
}
