package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// 颜色随终端深浅色自动切换（D-24）。
var (
	colorAccent = lipgloss.AdaptiveColor{Light: "#C2410C", Dark: "#FB923C"}
	colorDim    = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}
	colorError  = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	colorOK     = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	colorBar    = lipgloss.AdaptiveColor{Light: "#F3F4F6", Dark: "#1F2937"}
	colorSelBg  = lipgloss.AdaptiveColor{Light: "#FED7AA", Dark: "#7C2D12"}

	styleHeader   = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleDim      = lipgloss.NewStyle().Foreground(colorDim)
	styleSel      = lipgloss.NewStyle().Background(colorSelBg).Bold(true)
	styleSelBlur  = lipgloss.NewStyle().Foreground(colorAccent)
	styleGroup    = lipgloss.NewStyle().Bold(true)
	styleBar      = lipgloss.NewStyle().Background(colorBar)
	styleError    = lipgloss.NewStyle().Foreground(colorError)
	styleOK       = lipgloss.NewStyle().Foreground(colorOK)
	styleRank     = lipgloss.NewStyle().Foreground(colorDim)
	styleRankTop  = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleSepColor = lipgloss.NewStyle().Foreground(colorDim)
)

func (m *model) bodyHeight() int {
	h := m.height - 2
	if h < 1 {
		h = 1
	}
	return h
}

func (m *model) wide() bool { return m.width >= wideMinWidth }

func (m *model) leftWidth() int {
	w := m.width / 4
	if w < 18 {
		w = 18
	}
	if w > 30 {
		w = 30
	}
	return w
}

// paneAt 返回横坐标所在的栏。
func (m *model) paneAt(x int) (focus, bool) {
	if !m.wide() {
		return m.focus, true
	}
	lw := m.leftWidth()
	switch {
	case x < lw:
		return focusTree, true
	case x == lw:
		return 0, false
	}
	return focusList, true
}

func (m *model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.headerLine())
	b.WriteByte('\n')
	body := m.bodyLines()
	for _, l := range body {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	b.WriteString(m.statusLine())
	return b.String()
}

func (m *model) headerLine() string {
	title := "今日热榜"
	ctx := m.target.title
	if m.searchQ != "" {
		ctx = "搜索“" + m.searchQ + "”"
	}
	right := ""
	switch {
	case m.loading:
		right = "加载中…"
	case !m.updatedAt.IsZero():
		right = "更新于 " + m.updatedAt.Format("15:04")
	}
	left := " " + title
	if ctx != "" {
		left += "  ›  " + ctx
	}
	gap := m.width - runewidth.StringWidth(left) - runewidth.StringWidth(right) - 1
	if gap < 1 {
		left = runewidth.Truncate(left, m.width-runewidth.StringWidth(right)-2, "…")
		gap = m.width - runewidth.StringWidth(left) - runewidth.StringWidth(right) - 1
		if gap < 1 {
			return styleHeader.Render(pad(left, m.width))
		}
	}
	return styleHeader.Render(left) + strings.Repeat(" ", gap) + styleDim.Render(right) + " "
}

func (m *model) bodyLines() []string {
	h := m.bodyHeight()
	if m.help {
		lines := helpLines()
		for i, l := range lines {
			lines[i] = pad(l, m.width)
		}
		return fill(lines, m.width, h)
	}
	if !m.wide() {
		if m.focus == focusTree {
			return m.treeLines(m.width, h)
		}
		return m.listLines(m.width, h)
	}
	lw := m.leftWidth()
	left := m.treeLines(lw, h)
	right := m.listLines(m.width-lw-1, h)
	sep := styleSepColor.Render("│")
	out := make([]string, h)
	for i := range out {
		out[i] = left[i] + sep + right[i]
	}
	return out
}

func (m *model) treeLines(w, h int) []string {
	out := make([]string, 0, h)
	if len(m.rows) == 0 {
		out = append(out, styleDim.Render(pad(" 加载中…", w)))
	}
	for i := m.treeOff; i < len(m.rows) && len(out) < h; i++ {
		e := m.rows[i]
		icon := "  "
		if e.expandable {
			icon = "▸ "
			if e.expanded {
				icon = "▾ "
			}
		}
		text := pad(" "+strings.Repeat("  ", e.level)+icon+e.label, w)
		switch {
		case i == m.treeCur && m.focus == focusTree:
			text = styleSel.Render(text)
		case i == m.treeCur:
			text = styleSelBlur.Render(text)
		case e.kind == kindMenu || e.kind == kindFollowing:
			text = styleGroup.Render(text)
		}
		out = append(out, text)
	}
	return fill(out, w, h)
}

func (m *model) listLines(w, h int) []string {
	out := make([]string, 0, h)
	if len(m.items) == 0 {
		msg := " 请在左侧选择榜单"
		switch {
		case m.loading:
			msg = " 加载中…"
		case m.listNotice != "":
			msg = " " + m.listNotice
		case m.message != "" && m.isError:
			msg = " " + m.message
		case m.target.key() != "||" || m.searchQ != "":
			msg = " 暂无内容"
		}
		out = append(out, styleDim.Render(pad(msg, w)))
		return fill(out, w, h)
	}
	rankW := runewidth.StringWidth(itoa(int64(len(m.items))))
	// 右侧热度列按最宽的一项右对齐，最多占三分之一宽度。
	metaCol := 0
	for _, it := range m.items {
		if mw := runewidth.StringWidth(it.meta); mw > metaCol {
			metaCol = mw
		}
	}
	if metaCol > w/3 {
		metaCol = w / 3
	}
	for i := m.listOff; i < len(m.items) && len(out) < h; i++ {
		it := m.items[i]
		rank := strings.Repeat(" ", rankW-len(itoa(int64(i+1)))) + itoa(int64(i+1))
		metaW := metaCol
		meta := ""
		if metaW > 0 {
			meta = runewidth.Truncate(it.meta, metaW, "…")
			meta = strings.Repeat(" ", metaW-runewidth.StringWidth(meta)) + meta
		}
		titleW := w - rankW - 3 - metaW - 1
		if metaW == 0 {
			titleW = w - rankW - 3
		}
		if titleW < 4 {
			titleW, meta, metaW = w-rankW-3, "", 0
		}
		title := pad(runewidth.Truncate(it.title, titleW, "…"), titleW)
		var line string
		if i == m.listCur && m.focus == focusList {
			plain := " " + rank + "  " + title
			if metaW > 0 {
				plain += " " + meta
			}
			line = styleSel.Render(pad(plain, w))
		} else {
			rs := styleRank
			if i < 3 && m.searchQ == "" {
				rs = styleRankTop
			}
			t := title
			if i == m.listCur {
				t = styleSelBlur.Render(title)
			}
			line = " " + rs.Render(rank) + "  " + t
			used := 1 + rankW + 2 + titleW
			if metaW > 0 {
				line += " " + styleDim.Render(meta)
				used += 1 + metaW
			}
			if used < w {
				line += strings.Repeat(" ", w-used)
			}
		}
		out = append(out, line)
	}
	if m.listNotice != "" && len(out) < h {
		out = append(out, styleDim.Render(pad(" "+m.listNotice, w)))
	}
	return fill(out, w, h)
}

func (m *model) statusLine() string {
	if m.searching {
		return pad(m.input.View(), m.width)
	}
	var left string
	var leftStyle lipgloss.Style
	switch {
	case m.expired:
		left, leftStyle = "● 登录已失效", styleError
	case !m.loggedIn:
		left, leftStyle = "○ 未登录", styleDim
	case m.account == nil:
		left, leftStyle = "● 已登录", styleOK
	default:
		name := m.account.Email
		if name == "" {
			name = "已登录"
		}
		left, leftStyle = "● "+name+" · "+m.account.MemberText(), styleOK
	}
	mid := ""
	midStyle := styleDim
	switch {
	case m.message != "":
		mid = m.message
		if m.isError {
			midStyle = styleError
		}
	case m.newVer != "":
		mid, midStyle = "新版本 "+m.newVer+" 可用，运行 rebang update 升级", styleHeader
	}
	hints := "/ 搜索  ↵ 打开  y 复制  r 刷新  ? 帮助  q 退出"
	if !m.wide() {
		hints = "tab 切换  ? 帮助  q 退出"
	}
	lw := runewidth.StringWidth(left)
	hw := runewidth.StringWidth(hints)
	avail := m.width - lw - hw - 4
	if avail < 0 {
		hints, hw = "", 0
		avail = m.width - lw - 3
	}
	if avail < 0 {
		return styleBar.Render(pad(" "+left, m.width))
	}
	if runewidth.StringWidth(mid) > avail {
		mid = runewidth.Truncate(mid, avail, "…")
	}
	gap := avail - runewidth.StringWidth(mid)
	return styleBar.Render(" "+leftStyle.Render(left)+"  "+midStyle.Render(mid)+strings.Repeat(" ", gap)+styleDim.Render(hints)) + styleBar.Render(" ")
}

func helpLines() []string {
	return []string{
		"",
		" 快捷键",
		"",
		"   ↑ ↓ / j k      上下移动",
		"   ← → / h l      收起、展开或切换左右栏",
		"   tab            切换左右栏",
		"   回车           左栏：展开或打开榜单；右栏：在浏览器中打开",
		"   /              搜索，Esc 返回榜单",
		"   y              复制链接",
		"   r              刷新",
		"   g G            跳到开头、结尾",
		"   ?              显示或关闭帮助",
		"   q              退出",
		"",
		" 鼠标：点击选中，滚轮滚动，双击打开。",
		"",
		" 按任意键返回",
	}
}

// pad 按显示宽度截断并补齐到 w 列。
func pad(s string, w int) string {
	if w <= 0 {
		return ""
	}
	sw := runewidth.StringWidth(s)
	if sw > w {
		s = runewidth.Truncate(s, w, "…")
		sw = runewidth.StringWidth(s)
	}
	return s + strings.Repeat(" ", w-sw)
}

// fill 补齐到 h 行，每行 w 列。
func fill(lines []string, w, h int) []string {
	if len(lines) > h {
		lines = lines[:h]
	}
	blank := strings.Repeat(" ", w)
	for len(lines) < h {
		lines = append(lines, blank)
	}
	for i, l := range lines {
		if lipgloss.Width(l) < w {
			lines[i] = l + strings.Repeat(" ", w-lipgloss.Width(l))
		}
	}
	return lines
}
