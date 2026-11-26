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
		if strings.HasPrefix(val, `"`) {
			u, err := strconv.Unquote(val)
			if err != nil {
				return nil, "", fmt.Errorf("front matter: bad quoted value for %q: %w", key, err)
			}
			val = u
		}
		fields = append(fields, Field{Key: key, Value: val})
	}
}

// Format writes fields and body as a front matter document.
func Format(fields []Field, body string) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	for _, f := range fields {
		b.WriteString(f.Key + ": " + quoteIfNeeded(f.Value) + "\n")
	}
	b.WriteString("---\n")
	b.WriteString(body)
	return []byte(b.String())
}

func quoteIfNeeded(v string) string {
	if needsQuote(v) {
		return strconv.Quote(v)
	}
	return v
}

func needsQuote(v string) bool {
	if v == "" || v != strings.TrimSpace(v) {
		return true
	}
	if strings.Contains(v, ": ") || strings.Contains(v, " #") || strings.HasSuffix(v, ":") {
		return true
	}
	if strings.ContainsAny(v, "\n\r\t") {
		return true
	}
	return strings.ContainsRune("-?:,[]{}#&*!|>'\"%@`", rune(v[0]))
}
