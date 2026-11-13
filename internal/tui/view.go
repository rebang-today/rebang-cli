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
	styleSepColor = lipgloss.NewStyle().Foreground(colorSep)

	colorBrandBg   = lipgloss.AdaptiveColor{Light: "#C2410C", Dark: "#EA580C"}
	colorHoverBg   = lipgloss.AdaptiveColor{Light: "#F3F4F6", Dark: "#262626"}
	colorSep       = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#374151"}
	colorNoticeBg  = lipgloss.AdaptiveColor{Light: "#FFF7ED", Dark: "#431407"}
	styleBrand     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(colorBrandBg)
	styleCrumb     = lipgloss.NewStyle().Bold(true)
	styleMarker    = lipgloss.NewStyle().Foreground(colorAccent)
	styleHover     = lipgloss.NewStyle().Background(colorHoverBg)
	styleHoverText = lipgloss.NewStyle().Underline(true)
	styleLeaf      = lipgloss.NewStyle()
	styleKey       = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleSource    = lipgloss.NewStyle().Foreground(colorAccent)
	styleNotice    = lipgloss.NewStyle().Foreground(colorAccent).Background(colorNoticeBg)

	spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
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

// onBar 让样式带上状态栏背景，避免嵌套样式重置背景后出现断档。
func onBar(st lipgloss.Style) lipgloss.Style { return st.Background(colorBar) }

func barText(st lipgloss.Style, s string) string { return onBar(st).Render(s) }

func (m *model) spinner() string { return spinFrames[m.spin%len(spinFrames)] }

// crumbs 返回标题栏的位置：一级 Tab › 二级 Tab，或搜索词。
func (m *model) crumbs() []string {
	if m.searchQ != "" {
		return []string{"搜索“" + m.searchQ + "”"}
	}
	t := m.target
	if t.title == "" {
		return nil
	}
	if t.subTab != "" {
		for _, d := range m.tree.data {
			if d == nil {
				continue
			}
			for _, tab := range d.tabs {
				if tab.Key == t.tab {
					return []string{tab.Name, t.title}
				}
			}
		}
	}
	return []string{t.title}
}

func (m *model) headerLine() string {
	brand := styleBrand.Render(" 今日热榜 ")
	bw := lipgloss.Width(brand)
	// 右侧信息从详细到简略依次尝试，优先保证位置完整显示。
	var rights []string
	rightStyle := styleDim
	switch {
	case m.loading:
		rights, rightStyle = []string{m.spinner() + " 加载中", m.spinner()}, styleHeader
	case !m.updatedAt.IsZero():
		upd := "更新于 " + m.updatedAt.Format("15:04")
		if n := len(m.items); n > 0 {
			rights = append(rights, itoa(int64(n))+" 条 · "+upd)
		}
		rights = append(rights, upd, m.updatedAt.Format("15:04"))
	}
	rights = append(rights, "")
	cs := m.crumbs()
	plain := strings.Join(cs, " › ")
	right := ""
	for _, r := range rights {
		right = r
		if bw+2+runewidth.StringWidth(plain)+2+runewidth.StringWidth(r)+1 <= m.width {
			break
		}
	}
	avail := m.width - bw - 2 - runewidth.StringWidth(right) - 3
	crumb := ""
	if len(cs) > 0 && avail > 2 {
		if runewidth.StringWidth(plain) > avail {
			crumb = barText(styleCrumb, runewidth.Truncate(plain, avail, "…"))
		} else {
			var b strings.Builder
			for i, c := range cs {
				if i > 0 {
					b.WriteString(barText(styleDim, " › "))
				}
				st := styleDim
				if i == len(cs)-1 {
					st = styleCrumb
				}
				b.WriteString(barText(st, c))
			}
			crumb = b.String()
		}
	}
	gap := m.width - bw - 2 - lipgloss.Width(crumb) - runewidth.StringWidth(right) - 1
	if gap < 0 {
		gap = 0
	}
	line := brand + barText(styleBar, "  ") + crumb + barText(styleBar, strings.Repeat(" ", gap)) + barText(rightStyle, right) + barText(styleBar, " ")
	if w := lipgloss.Width(line); w < m.width {
		line += barText(styleBar, strings.Repeat(" ", m.width-w))
	}
	return line
}

func (m *model) bodyLines() []string {
	h := m.bodyHeight()
	if m.help {
		return m.helpBody(m.width, h)
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

// marker 是行首的选中标记：当前栏选中时为橙色竖条。
func marker(selected, focused bool) string {
	if selected && focused {
		return styleMarker.Render("▌")
	}
	return " "
}

func (m *model) treeLines(w, h int) []string {
	out := make([]string, 0, h)
	if len(m.rows) == 0 {
		if m.menuErr != "" {
			return centered([]string{styleError.Render("✗ 菜单加载失败"), "", keyHint("r", "重试")}, w, h)
		}
		return centered([]string{styleHeader.Render(m.spinner()) + styleDim.Render(" 正在加载菜单…")}, w, h)
	}
	focused := m.focus == focusTree
	for i := m.treeOff; i < len(m.rows) && len(out) < h; i++ {
		e := m.rows[i]
		icon := "  "
		if e.expandable {
			icon = "▸ "
			if e.expanded {
				icon = "▾ "
			}
		}
		sel := i == m.treeCur
		text := pad(strings.Repeat("  ", e.level)+icon+e.label, w-1)
		switch {
		case sel && focused:
			text = styleSel.Render(text)
		case sel:
			text = styleSelBlur.Render(text)
		case m.hovered(focusTree, i):
			text = styleHover.Render(text)
		case e.kind == kindMenu || e.kind == kindFollowing:
			text = styleGroup.Render(text)
		case !e.expandable && e.level > 0:
			text = styleLeaf.Render(text)
		}
		out = append(out, marker(sel, focused)+text)
	}
	return fill(out, w, h)
}

// emptyState 返回右栏没有条目时的提示（图标、说明、下一步）。
func (m *model) emptyState(w int) []string {
	wrap := func(st lipgloss.Style, s string) []string {
		lim := w - 4
		if lim < 8 {
			lim = w
		}
		var out []string
		for _, l := range strings.Split(runewidth.Wrap(s, lim), "\n") {
			out = append(out, st.Render(l))
		}
		return out
	}
	switch {
	case m.loading:
		return []string{styleHeader.Render(m.spinner()) + " 正在加载…"}
	case m.listErr != "":
		lines := append([]string{styleError.Render("✗ 加载失败"), ""}, wrap(styleDim, m.listErr)...)
		return append(lines, "", keyHint("r", "重试"))
	case m.listNotice != "":
		lines := []string{styleDim.Render(m.listNotice)}
		if m.searchQ != "" {
			lines = append(lines, "", keyHint("/", "换个关键词")+"   "+keyHint("esc", "返回榜单"))
		}
		return lines
	case m.target.key() != "||" || m.searchQ != "":
		return []string{styleDim.Render("暂无内容"), "", keyHint("r", "刷新")}
	case m.menuErr != "":
		return []string{styleDim.Render(m.menuErr)}
	}
	return []string{styleDim.Render("← 在左侧选择一个榜单"), "", keyHint("↑↓", "移动") + "   " + keyHint("↵", "打开")}
}

// centered 把几行文字放到区域中间。
func centered(lines []string, w, h int) []string {
	out := make([]string, 0, h)
	top := (h - len(lines)) / 3
	for i := 0; i < top; i++ {
		out = append(out, "")
	}
	for _, l := range lines {
		lw := lipgloss.Width(l)
		if lw > w {
			out = append(out, pad(l, w))
			continue
		}
		out = append(out, strings.Repeat(" ", (w-lw)/2)+l)
	}
	return fill(out, w, h)
}

func (m *model) listLines(w, h int) []string {
	if len(m.items) == 0 {
		return centered(m.emptyState(w), w, h)
	}
	out := make([]string, 0, h)
	focused := m.focus == focusList
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
		// 行首标记 1 列 + 排名 + 两个空格 + 标题 + 空格 + 热度 + 行尾 1 列。
		titleW := w - 1 - rankW - 2 - metaW - 2
		if metaW == 0 {
			titleW = w - 1 - rankW - 3
		}
		if titleW < 4 {
			titleW, meta, metaW = w-1-rankW-3, "", 0
		}
		short := runewidth.Truncate(it.title, titleW, "…")
		title := pad(short, titleW)
		sel := i == m.listCur
		var line string
		if sel && focused {
			plain := rank + "  " + title
			if metaW > 0 {
				plain += " " + meta
			}
			line = styleSel.Render(pad(plain, w-1))
		} else {
			rs := styleRank
			if i < 3 && m.searchQ == "" {
				rs = styleRankTop
			}
			ts := lipgloss.NewStyle()
			switch {
			case sel:
				ts = styleSelBlur
			case m.hovered(focusList, i):
				ts = styleHoverText
			}
			// 只给文字本身上样式，避免下划线延伸到行尾空白。
			line = rs.Render(rank) + "  " + ts.Render(short) + strings.Repeat(" ", titleW-runewidth.StringWidth(short))
			used := rankW + 2 + titleW
			if metaW > 0 {
				ms := styleDim
				if m.searchQ != "" {
					ms = styleSource
				}
				line += " " + ms.Render(meta)
				used += 1 + metaW
			}
			if used < w-1 {
				line += strings.Repeat(" ", w-1-used)
			}
		}
		out = append(out, marker(sel, focused)+line)
	}
	if m.listNotice != "" && len(out) < h {
		out = append(out, "", styleNotice.Render(pad(" ★ "+m.listNotice, w)))
	}
	return fill(out, w, h)
}

// keyHint 渲染“按键 说明”形式的提示。
func keyHint(key, label string) string {
	return styleKey.Render(key) + " " + styleDim.Render(label)
}

func barHint(key, label string) string {
	return barText(styleKey, key) + barText(styleDim, " "+label)
}

type hint struct{ key, label string }

func (m *model) hints() []hint {
	if !m.wide() {
		if m.focus == focusTree {
			return []hint{{"↵", "打开"}, {"tab", "列表"}, {"?", "帮助"}, {"q", "退出"}}
		}
		return []hint{{"↵", "打开"}, {"tab", "目录"}, {"/", "搜索"}, {"?", "帮助"}, {"q", "退出"}}
	}
	if m.searchQ != "" {
		return []hint{{"↵", "打开"}, {"y", "复制"}, {"/", "搜索"}, {"esc", "返回"}, {"?", "帮助"}, {"q", "退出"}}
	}
	return []hint{{"/", "搜索"}, {"↵", "打开"}, {"y", "复制"}, {"r", "刷新"}, {"?", "帮助"}, {"q", "退出"}}
}

func (m *model) statusLine() string {
	if m.searching {
		return pad(m.input.View(), m.width)
	}
	var left string
	var leftStyle lipgloss.Style
	switch {
	case m.expired:
		left, leftStyle = "● 登录已失效，退出后运行 rebang auth login", styleError
	case !m.loggedIn:
		left, leftStyle = "○ 未登录", styleDim
	case m.account == nil:
		left, leftStyle = "● 已登录", styleOK
	default:
		name := m.account.Email
		if name == "" {
			name = "已登录"
		}
		left, leftStyle = "● "+name, styleOK
		if mt := m.account.MemberText(); mt != "" {
			left += " · " + mt
		}
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
		mid, midStyle = "↑ 新版本 "+m.newVer+"，运行 rebang update 升级", styleHeader
	}
	// 按宽度从右往左丢弃提示，保证状态与消息可见。
	hs := m.hints()
	render := func(hs []hint) (string, int) {
		parts := make([]string, len(hs))
		plain := 0
		for i, h := range hs {
			parts[i] = barHint(h.key, h.label)
			plain += runewidth.StringWidth(h.key) + 1 + runewidth.StringWidth(h.label)
		}
		if len(hs) > 1 {
			plain += 2 * (len(hs) - 1)
		}
		return strings.Join(parts, barText(styleBar, "  ")), plain
	}
	lw := runewidth.StringWidth(left)
	if lw > m.width-2 {
		left = runewidth.Truncate(left, m.width-2, "…")
		lw = runewidth.StringWidth(left)
	}
	hintStr, hw := render(hs)
	// 有消息时至少给它留 24 列（或消息本身的宽度），其余空间给按键提示。
	minMid := runewidth.StringWidth(mid)
	if minMid > 24 {
		minMid = 24
	}
	for len(hs) > 0 && m.width-lw-hw-6-minMid < 0 {
		hs = hs[:len(hs)-1]
		hintStr, hw = render(hs)
	}
	if len(hs) == 0 {
		hw = -2
	}
	avail := m.width - lw - hw - 6
	if avail < 0 {
		avail = 0
	}
	if runewidth.StringWidth(mid) > avail {
		mid = runewidth.Truncate(mid, avail, "…")
	}
	gap := avail - runewidth.StringWidth(mid)
	line := barText(styleBar, " ") + barText(leftStyle, left) + barText(styleBar, "  ") + barText(midStyle, mid) + barText(styleBar, strings.Repeat(" ", gap)+"  ") + hintStr
	if w := lipgloss.Width(line); w < m.width {
		line += barText(styleBar, strings.Repeat(" ", m.width-w))
	}
	return line
}

var helpKeys = []struct {
	group string
	keys  []hint
}{
	{"浏览", []hint{{"↑ ↓  j k", "上下移动"}, {"← →  h l", "收起、展开或切换左右栏"}, {"tab", "切换左右栏"}, {"g  G", "跳到开头、结尾"}}},
	{"操作", []hint{{"回车", "左栏展开或打开榜单；右栏在浏览器中打开"}, {"/", "搜索，Esc 返回榜单"}, {"y", "复制链接"}, {"r", "刷新"}}},
	{"其他", []hint{{"?", "显示或关闭帮助"}, {"q", "退出"}}},
}

func (m *model) helpBody(w, h int) []string {
	lines := []string{styleHeader.Render("快捷键"), ""}
	kw := 0
	for _, g := range helpKeys {
		for _, k := range g.keys {
			if x := runewidth.StringWidth(k.key); x > kw {
				kw = x
			}
		}
	}
	for _, g := range helpKeys {
		lines = append(lines, styleGroup.Render(g.group))
		for _, k := range g.keys {
			lines = append(lines, "  "+styleKey.Render(runewidth.FillRight(k.key, kw))+"   "+k.label)
		}
		lines = append(lines, "")
	}
	lines = append(lines, styleDim.Render("鼠标：点击选中，双击打开，滚轮滚动"), "", styleDim.Render("按任意键返回"))
	// 窗口不够高时去掉空行。
	if len(lines) > h {
		compact := lines[:0:0]
		for _, l := range lines {
			if l != "" {
				compact = append(compact, l)
			}
		}
		lines = compact
	}
	// 整体左对齐，块居中。
	bw := 0
	for _, l := range lines {
		if x := lipgloss.Width(l); x > bw {
			bw = x
		}
	}
	left := (w - bw) / 2
	if left < 1 {
		left = 1
	}
	top := (h - len(lines)) / 2
	if top < 0 {
		top = 0
	}
	out := make([]string, 0, h)
	for i := 0; i < top; i++ {
		out = append(out, "")
	}
	for _, l := range lines {
		if lipgloss.Width(l)+left > w {
			l = runewidth.Truncate(l, w-left, "…")
		}
		out = append(out, strings.Repeat(" ", left)+l)
	}
	return fill(out, w, h)
}

// helpLines 返回纯文本的帮助内容，供测试检查。
func helpLines() []string {
	var out []string
	for _, g := range helpKeys {
		for _, k := range g.keys {
			out = append(out, k.key+" "+k.label)
		}
	}
	return out
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
