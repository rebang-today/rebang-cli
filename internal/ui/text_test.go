package ui

import (
	"testing"
	"unicode"
)

func TestCleanText(t *testing.T) {
	invisible := "\u200b\u200c\u200d\u200e\u200f\u202a\u202b\u202c\u202d\u202e\u2060\u2061\u2062\u2063\u2064\u2066\u2067\u2068\u2069\ufeff\u00ad\x00\x07\x1b\x7f"
	cases := map[string]string{
		"":     "",
		"普通标题": "普通标题",
		invisible + "爱范儿" + invisible + "标题": "爱范儿标题",
		"  a\n\tb\r\nc  ":     "a b c",
		"a\u2028b   c\u0085d": "a b c d",
		"\x1b[31m红\x1b[0m":    "[31m红[0m",
	}
	for in, want := range cases {
		got := CleanText(in)
		if got != want {
			t.Errorf("CleanText(%q) = %q, 想要 %q", in, got, want)
		}
		for _, r := range got {
			if unicode.In(r, unicode.Cf, unicode.Cc) {
				t.Errorf("CleanText(%q) 仍含 %U", in, r)
			}
		}
	}
}
