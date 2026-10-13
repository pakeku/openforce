package main

import "strings"

func splitSQL(input string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		stmt := strings.TrimSpace(b.String())
		b.Reset()
		if stmt != "" {
			out = append(out, stmt)
		}
	}

	i := 0
	n := len(input)
	for i < n {
		if strings.HasPrefix(input[i:], "--") {
			if nl := strings.IndexByte(input[i:], '\n'); nl >= 0 {
				i += nl + 1
			} else {
				break
			}
			continue
		}
		if strings.HasPrefix(input[i:], "/*") {
			if end := strings.Index(input[i+2:], "*/"); end >= 0 {
				i += end + 4
			} else {
				break
			}
			continue
		}

		c := input[i]
		if c == '\'' || c == '"' {
			b.WriteByte(c)
			i++
			for i < n {
				b.WriteByte(input[i])
				if input[i] == c {
					i++
					if i < n && input[i] == c {
						b.WriteByte(input[i])
						i++
						continue
					}
					break
				}
				i++
			}
			continue
		}
		if c == '$' {
			if tag, ok := dollarTag(input[i:]); ok {
				end := strings.Index(input[i+len(tag):], tag)
				if end < 0 {
					b.WriteString(input[i:])
					break
				}
				end = i + len(tag) + end + len(tag)
				b.WriteString(input[i:end])
				i = end
				continue
			}
		}
		if c == ';' {
			flush()
			i++
			continue
		}
		b.WriteByte(c)
		i++
	}
	flush()
	return out
}

func dollarTag(s string) (string, bool) {
	if len(s) < 2 || s[0] != '$' {
		return "", false
	}
	if s[1] == '$' {
		return "$$", true
	}
	if !isTagStart(s[1]) {
		return "", false
	}
	for i := 2; i < len(s); i++ {
		if s[i] == '$' {
			return s[:i+1], true
		}
		if !isTagChar(s[i]) {
			return "", false
		}
	}
	return "", false
}

func isTagStart(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isTagChar(c byte) bool {
	return isTagStart(c) || (c >= '0' && c <= '9')
}
