package util

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestTruncate(t *testing.T) {
	t.Run("short string is unchanged", func(t *testing.T) {
		assert.Equal(t, "hello", Truncate("hello", 10))
	})

	t.Run("ascii is cut at limit", func(t *testing.T) {
		assert.Equal(t, "hel...", Truncate("hello", 3))
	})

	t.Run("multibyte rune crossing the limit is not split", func(t *testing.T) {
		s := strings.Repeat("a", 999) + "é"
		got := Truncate(s, 1000)
		assert.True(t, utf8.ValidString(got))
		assert.Equal(t, strings.Repeat("a", 999)+"...", got)
	})

	t.Run("invalid utf8 in short output is replaced", func(t *testing.T) {
		got := Truncate("ab\xffcd", 100)
		assert.True(t, utf8.ValidString(got))
		assert.Equal(t, "ab�cd", got)
	})
}
