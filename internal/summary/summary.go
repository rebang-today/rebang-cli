// Package summary 把 AI 总结整理成可显示的行，供命令行与交互界面共用（D-26）。
//
// 分区按“要点 → 经过 → 观点 → 网友怎么看”的顺序展示，缺失的分区跳过，不认识的分区忽略。
// 文本中可以有一处 **…** 渲染为加粗，其余 Markdown 原样显示；原话（quote）按引用样式显示。
package summary

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/ui"
)

// Kind 是一行内容的类别。
type Kind int

const (
	// Blank 空行，用于分隔分区。
	Blank Kind = iota
	// Heading 分区标题。
	Heading
	// Para 正文段落。
	Para
	// Bullet 列表项，以 “• ” 开头，换行后悬挂缩进。
	Bullet
	// Quote 原话，引用样式。
	Quote
	// Byline 署名等次要信息。
	Byline
)

// Line 是一行逻辑内容，显示时按宽度折行。
type Line struct {
	Kind   Kind
	Indent int
	// Label 显示在正文前的标签（时间、观点概括、说话的人），使用标签样式。
	Label string
	Text  string
}

// 分区标题。
const (
	TitlePoints     = "要点"
	TitleTimeline   = "经过"
	TitleViewpoints = "观点"
	TitleReactions  = "网友怎么看"
)

// Sections 按固定顺序生成各分区的行，不含一句话总结。没有任何分区时返回 nil。
func Sections(d api.InsightDetail) []Line {
	var out []Line
	section := func(title string, lines []Line) {
		if len(lines) == 0 {
			return
		}
		if len(out) > 0 {
			out = append(out, Line{Kind: Blank})
		}
		out = append(out, Line{Kind: Heading, Text: title})
		out = append(out, lines...)
	}

	var pts []Line
	for _, p := range d.Points {
		if p = ui.CleanText(p); p != "" {
			pts = append(pts, Line{Kind: Bullet, Text: p})
		}
	}
	section(TitlePoints, pts)

	var tl []Line
	for _, e := range d.Timeline {
		text := ui.CleanText(e.Text)
		if text == "" {
			continue
		}
		tl = append(tl, Line{Kind: Bullet, Label: ui.CleanText(e.Time), Text: text})
	}
	section(TitleTimeline, tl)

	var vp []Line
	for _, v := range d.Viewpoints {
		stance, text := ui.CleanText(v.Stance), ui.CleanText(v.Text)
		if stance == "" && text == "" {
			continue
		}
		if stance == "" {
			vp = append(vp, Line{Kind: Bullet, Text: text})
		} else {
			vp = append(vp, Line{Kind: Bullet, Label: stance})
			if text != "" {
				vp = append(vp, Line{Kind: Para, Indent: 2, Text: text})
			}
		}
		if q := ui.CleanText(v.Quote); q != "" {
			vp = append(vp, Line{Kind: Quote, Indent: 2, Text: q})
		}
		if by := ui.CleanText(v.By); by != "" {
			vp = append(vp, Line{Kind: Byline, Indent: 2, Text: "—— " + by})
		}
	}
	section(TitleViewpoints, vp)

	if r := d.Reactions; r != nil {
		var rl []Line
		if mood := ui.CleanText(r.Mood); mood != "" {
			rl = append(rl, Line{Kind: Para, Text: mood})
		}
		for _, v := range r.Voices {
			by, text := ui.CleanText(v.By), ui.CleanText(v.Text)
			if text == "" && by == "" {
				continue
			}
			l := Line{Kind: Bullet, Text: text}
			if by != "" {
				l.Label = by + "："
				if text == "" {
					l.Label = by
				}
			}
			rl = append(rl, l)
			if q := ui.CleanText(v.Quote); q != "" {
				rl = append(rl, Line{Kind: Quote, Indent: 2, Text: q})
			}
		}
		section(TitleReactions, rl)
	}
	return out
}

// Span 是一段同样式的文本。
type Span struct {
	Text string
	Bold bool
}

// ParseBold 把第一处成对的 **…** 标为加粗；标记不成对或内容为空时原样保留。
func ParseBold(s string) []Span {
	i := strings.Index(s, "**")
	if i < 0 {
		return []Span{{Text: s}}
	}
	j := strings.Index(s[i+2:], "**")
	if j <= 0 {
		return []Span{{Text: s}}
	}
	var out []Span
	if i > 0 {
		out = append(out, Span{Text: s[:i]})
	}
	out = append(out, Span{Text: s[i+2 : i+2+j], Bold: true})
	if rest := s[i+4+j:]; rest != "" {
		out = append(out, Span{Text: rest})
	}
	return out
}

// Style 决定各类文本的显示样式，nil 表示原样输出。
type Style struct {
	Heading func(string) string
	Label   func(string) string
	Bold    func(string) string
	Quote   func(string) string
	Dim     func(string) string
}

type role int

const (
	rolePlain role = iota
	roleBold
	roleLabel
	roleQuote
	roleDim
	roleHeading
)

func (s Style) apply(r role, t string) string {
	var f func(string) string
	switch r {
	case roleBold:
		f = s.Bold
	case roleLabel:
		f = s.Label
	case roleQuote:
		f = s.Quote
	case roleDim:
		f = s.Dim
	case roleHeading:
		f = s.Heading
	}
	if f == nil || t == "" {
		return t
	}
	return f(t)
}

type seg struct {
	text string
	role role
}

// Render 把逻辑行按显示宽度折行并套用样式。返回的每行显示宽度不超过 width。
func Render(lines []Line, width int, st Style) []string {
	if width <= 0 {
		return nil
	}
	var out []string
	for _, l := range lines {
		indent := min(l.Indent, width-1)
		if indent < 0 {
			indent = 0
		}
		var segs []seg
		first, hang := spaces(indent), spaces(indent)
		switch l.Kind {
		case Blank:
			out = append(out, "")
			continue
		case Heading:
			segs = []seg{{l.Text, roleHeading}}
		case Quote:
			segs = []seg{{"“" + l.Text + "”", roleQuote}}
		case Byline:
			segs = []seg{{l.Text, roleDim}}
		case Bullet:
			if indent+2 < width {
				first, hang = first+"• ", hang+"  "
			}
			fallthrough
		default:
			if l.Label != "" {
				segs = append(segs, seg{l.Label, roleLabel})
				if l.Text != "" && !strings.HasSuffix(l.Label, "：") {
					segs = append(segs, seg{" ", rolePlain})
				}
			}
			for _, sp := range ParseBold(l.Text) {
				r := rolePlain
				if sp.Bold {
					r = roleBold
				}
				segs = append(segs, seg{sp.Text, r})
			}
		}
		for i, wl := range wrapSegs(segs, width-runewidth.StringWidth(first)) {
			p := hang
			if i == 0 {
				p = first
			}
			var b strings.Builder
			b.WriteString(p)
			for _, s := range wl {
				b.WriteString(st.apply(s.role, s.text))
			}
			out = append(out, b.String())
		}
	}
	return out
}

// Plain 返回不带样式的折行结果，便于测试与纯文本输出。
func Plain(lines []Line, width int) []string { return Render(lines, width, Style{}) }

// atom 是折行的最小单位：一个汉字或一串连续的 ASCII 非空白字符。
type atom struct {
	text  string
	role  role
	width int
	space bool
}

func atoms(segs []seg) []atom {
	var out []atom
	for _, s := range segs {
		t := s.text
		for len(t) > 0 {
			r, n := utf8.DecodeRuneInString(t)
			if r < utf8.RuneSelf && !unicode.IsSpace(r) {
				// 连续 ASCII 单词作为整体，过长时再按字符拆开。
				j := n
				for j < len(t) {
					r2, n2 := utf8.DecodeRuneInString(t[j:])
					if r2 >= utf8.RuneSelf || unicode.IsSpace(r2) {
						break
					}
					j += n2
				}
				out = append(out, atom{text: t[:j], role: s.role, width: j})
				t = t[j:]
				continue
			}
			out = append(out, atom{text: t[:n], role: s.role, width: runewidth.RuneWidth(r), space: unicode.IsSpace(r)})
			t = t[n:]
		}
	}
	return out
}

func wrapSegs(segs []seg, width int) [][]seg {
	if width <= 0 {
		return [][]seg{nil}
	}
	var lines [][]seg
	var cur []seg
	used := 0
	push := func(a atom) {
		if n := len(cur); n > 0 && cur[n-1].role == a.role {
			cur[n-1].text += a.text
		} else {
			cur = append(cur, seg{a.text, a.role})
		}
		used += a.width
	}
	flush := func() {
		// 行尾空白去掉。
		if n := len(cur); n > 0 {
			cur[n-1].text = strings.TrimRight(cur[n-1].text, " ")
		}
		lines = append(lines, cur)
		cur, used = nil, 0
	}
	for _, a := range atoms(segs) {
		if a.space {
			if used == 0 {
				continue
			}
			if used+a.width > width {
				flush()
				continue
			}
			push(a)
			continue
		}
		if used+a.width <= width {
			push(a)
			continue
		}
		if a.width <= width && used > 0 {
			flush()
			push(a)
			continue
		}
		// 单个单词比整行还宽：按字符拆开。
		for _, r := range a.text {
			rw := runewidth.RuneWidth(r)
			if rw > width {
				continue
			}
			if used+rw > width {
				flush()
			}
			push(atom{text: string(r), role: a.role, width: rw})
		}
	}
	if len(cur) > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}
