package util

import (
	"strings"
	"unicode/utf8"
)

func Truncate(s string, limit int) string {
	s = strings.ToValidUTF8(s, string(utf8.RuneError))
	if len(s) <= limit {
		return s
	}

	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}
