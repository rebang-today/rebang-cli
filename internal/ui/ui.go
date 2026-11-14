// Package ui 提供命令行（非交互界面）的人类可读样式：配色、键值卡片、状态行。
//
// 只在输出连接到终端且未设置 NO_COLOR 时上色；否则所有样式退化为纯文本，
// 保证管道与重定向时输出稳定、可被其他程序处理。
package ui

import (
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
)

// ColorEnabled 判断是否可以输出颜色：连接到终端、NO_COLOR 未设置或为空（https://no-color.org）、TERM 不是 dumb。
func ColorEnabled(isTTY bool) bool {
	if !isTTY {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return os.Getenv("TERM") != "dumb"
}

// 与交互界面一致的配色（强调色为网站蓝，前 3 名序号为网站橙）。使用 256 色与终端基础色，深浅背景下都可读。
const (
	colorAccent  = lipgloss.Color("33")  // 蓝，接近网站主题色 #2C5CF5，深浅背景都可读
	colorRankTop = lipgloss.Color("202") // 橙，网站前 3 名序号的颜色 #F7670D
	colorOK      = lipgloss.Color("2")
	colorWarn    = lipgloss.Color("3")
	colorErr     = lipgloss.Color("1")
	colorDim     = lipgloss.Color("244")
	colorBorder  = lipgloss.Color("240")
)

// Theme 是一组样式。零值不可用，用 New 创建。
type Theme struct {
	Color bool

	Accent  lipgloss.Style
	Strong  lipgloss.Style
	OK      lipgloss.Style
	Warn    lipgloss.Style
	Err     lipgloss.Style
	Dim     lipgloss.Style
	Label   lipgloss.Style
	Code    lipgloss.Style
	Border  lipgloss.Style
	TableHd lipgloss.Style
	// RankTop 是表格中前 3 名序号的样式。
	RankTop lipgloss.Style

	r *lipgloss.Renderer
}

// New 创建主题。color 为 false 时所有样式都是纯文本。
func New(color bool) Theme {
	r := lipgloss.NewRenderer(io.Discard)
	if color {
		r.SetColorProfile(termenv.ANSI256)
	} else {
		r.SetColorProfile(termenv.Ascii)
	}
	s := r.NewStyle
	t := Theme{Color: color, r: r}
	t.Accent, t.Strong, t.OK, t.Warn, t.Err, t.Dim, t.Label, t.Code, t.Border, t.TableHd, t.RankTop =
		s(), s(), s(), s(), s(), s(), s(), s(), s(), s(), s()
	if color {
		t.Accent = s().Foreground(colorAccent)
		t.Strong = s().Bold(true)
		t.OK = s().Foreground(colorOK).Bold(true)
		t.Warn = s().Foreground(colorWarn)
		t.Err = s().Foreground(colorErr).Bold(true)
		t.Dim = s().Foreground(colorDim)
		t.Label = s().Foreground(colorDim)
		t.Code = s().Foreground(colorAccent).Bold(true)
		t.Border = s().Foreground(colorBorder)
		t.TableHd = s().Foreground(colorDim).Bold(true)
		t.RankTop = s().Foreground(colorRankTop).Bold(true)
	}
	return t
}

// Plain 是不带颜色的主题。
func Plain() Theme { return New(false) }

// 状态符号。
const (
	MarkOK   = "✓"
	MarkErr  = "✗"
	MarkWarn = "!"
	MarkOn   = "●"
	MarkOff  = "○"
)

// Success 返回成功行，例如“✓ 登录成功”。
func (t Theme) Success(msg string) string { return t.OK.Render(MarkOK) + " " + t.Strong.Render(msg) }

// Failure 返回失败行。
func (t Theme) Failure(msg string) string { return t.Err.Render(MarkErr + " " + msg) }

// Warning 返回警告行。
func (t Theme) Warning(msg string) string { return t.Warn.Render(MarkWarn) + " " + msg }

// Field 是键值卡片中的一行。Tone 为空时使用默认样式。
type Field struct {
	Key   string
	Value string
	Tone  Tone
}

// Tone 指定值的语气。
type Tone int

const (
	ToneNormal Tone = iota
	ToneStrong
	ToneOK
	ToneWarn
	ToneErr
	ToneDim
	ToneAccent
)

func (t Theme) tone(tn Tone) lipgloss.Style {
	switch tn {
	case ToneStrong:
		return t.Strong
	case ToneOK:
		return t.OK
	case ToneWarn:
		return t.Warn
	case ToneErr:
		return t.Err
	case ToneDim:
		return t.Dim
	case ToneAccent:
		return t.Accent
	}
	return t.r.NewStyle()
}

// KV 渲染左对齐的键值列表，键按显示宽度对齐（正确处理中文）。值为空的行会被跳过。
func (t Theme) KV(fields []Field, indent int) string {
	kw := 0
	for _, f := range fields {
		if f.Value == "" {
			continue
		}
		if w := runewidth.StringWidth(f.Key); w > kw {
			kw = w
		}
	}
	pad := strings.Repeat(" ", indent)
	var b strings.Builder
	for _, f := range fields {
		if f.Value == "" {
			continue
		}
		b.WriteString(pad)
		b.WriteString(t.Label.Render(runewidth.FillRight(f.Key, kw)))
		b.WriteString("   ")
		b.WriteString(t.tone(f.Tone).Render(f.Value))
		b.WriteByte('\n')
	}
	return b.String()
}

// Card 用圆角边框包裹内容。maxWidth 为终端宽度，0 表示不限制；终端太窄时不画边框。
func (t Theme) Card(body string, maxWidth int) string {
	body = strings.TrimRight(body, "\n")
	inner := lipgloss.Width(body)
	if maxWidth > 0 && inner+4 > maxWidth {
		return body + "\n"
	}
	box := t.r.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1)
	if !t.Color {
		box = t.r.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	}
	return box.Render(body) + "\n"
}

// CodeBox 突出显示授权码等需要用户核对的短文本。
func (t Theme) CodeBox(code string) string {
	box := t.r.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 2)
	if t.Color {
		box = box.BorderForeground(colorAccent)
	}
	return box.Render(t.Code.Render(spaced(code)))
}

// spaced 在授权码字符之间加空格，便于核对（ABCD-EFGH → A B C D - E F G H）。
func spaced(s string) string {
	rs := []rune(s)
	if len(rs) > 16 {
		return s
	}
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = string(r)
	}
	return strings.Join(parts, " ")
}

// Hint 返回“下一步”提示行，命令部分高亮。
func (t Theme) Hint(text, cmd string) string {
	if cmd == "" {
		return t.Dim.Render(text)
	}
	return t.Dim.Render(text) + t.Accent.Render(cmd)
}

// Width 返回去除样式后的显示宽度。
func Width(s string) int { return lipgloss.Width(s) }
