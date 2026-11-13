package ui

import (
	"strings"
	"unicode"
)

// CleanText 清洗要显示到终端的外部文本（标题、描述、来源名等）：
// 去掉 Unicode 格式字符（Cf，如零宽字符、双向文本控制符、U+FEFF、软连字符）
// 和控制字符（Cc），换行、制表符等空白替换为空格，连续空白折叠为一个，并去掉首尾空白。
//
// 这些字符在不同终端中的宽度与双向处理不一致，原样输出会导致整行溢出、界面错位。
// 只用于给人看的输出，JSON 保留接口原文。
func CleanText(s string) string {
	clean := true
	for _, r := range s {
		if r == ' ' || (r > 0x20 && r < 0x7f) {
			continue
		}
		if r < 0x20 || r == 0x7f || unicode.IsSpace(r) || unicode.In(r, unicode.Cf, unicode.Cc) || r == unicode.ReplacementChar {
			clean = false
			break
		}
	}
	if clean && !strings.Contains(s, "  ") && strings.TrimSpace(s) == s {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			space = true
		case unicode.In(r, unicode.Cf, unicode.Cc):
			// 丢弃。
		default:
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		}
	}
	return b.String()
}
