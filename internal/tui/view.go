package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// 颜色随终端深浅色自动切换（D-24）。浅色模式用网站的蓝色（--primary）；深色模式下网站蓝
// 饱和度过高、发闷，改用更亮更柔和的 #7AA2F7 及同色调深蓝背景（方案 B）。
// 前 3 名序号用网站的橙色（--rank-top）。
var (
	colorAccent = lipgloss.AdaptiveColor{Light: "#2C5CF5", Dark: "#7AA2F7"}
	colorDim    = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}
	colorError  = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}
	colorOK     = lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#4ADE80"}
	colorBar    = lipgloss.AdaptiveColor{Light: "#F3F4F6", Dark: "#1F2937"}
	colorSelBg  = lipgloss.AdaptiveColor{Light: "#E0E8FE", Dark: "#283457"}
	// 浅色背景上 #F7670D 对比度只有 3:1，改用略深的橙色。
	colorRankTop = lipgloss.AdaptiveColor{Light: "#D9480F", Dark: "#F7670D"}

	styleHeader   = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleDim      = lipgloss.NewStyle().Foreground(colorDim)
	styleSel      = lipgloss.NewStyle().Background(colorSelBg).Bold(true)
	styleSelBlur  = lipgloss.NewStyle().Bold(true)
	styleGroup    = lipgloss.NewStyle().Bold(true)
	styleBar      = lipgloss.NewStyle().Background(colorBar)
	styleError    = lipgloss.NewStyle().Foreground(colorError)
	styleOK       = lipgloss.NewStyle().Foreground(colorOK)
	styleRank     = lipgloss.NewStyle().Foreground(colorDim)
	styleRankTop  = lipgloss.NewStyle().Foreground(colorRankTop).Bold(true)
	styleSepColor = lipgloss.NewStyle().Foreground(colorSep)

	colorBrandBg   = lipgloss.AdaptiveColor{Light: "#2C5CF5", Dark: "#3D63D8"} // 白字对比度均约 5.3:1
	colorHoverBg   = lipgloss.AdaptiveColor{Light: "#F3F4F6", Dark: "#262626"}
	colorSep       = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#374151"}
	colorNoticeBg  = lipgloss.AdaptiveColor{Light: "#EEF2FF", Dark: "#1F2A44"}
	styleBrand     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(colorBrandBg)
	styleCrumb     = lipgloss.NewStyle().Bold(true)
	styleMarker    = lipgloss.NewStyle().Foreground(colorAccent)
	styleHover     = lipgloss.NewStyle().Background(colorHoverBg)
	styleHoverText = lipgloss.NewStyle().Underline(true)
	styleKey       = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleSource    = lipgloss.NewStyle().Foreground(colorAccent)
	styleNotice    = lipgloss.NewStyle().Foreground(colorAccent).Background(colorNoticeBg)
	// 菜单栏与子榜单条：当前项用浅蓝底，其余为普通文字。
	styleMenu   = lipgloss.NewStyle()
	styleMenuOn = lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Background(colorTabBg)
	styleTabOn  = lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Background(colorTabBg)
	colorTabBg  = lipgloss.AdaptiveColor{Light: "#EEF2FF", Dark: "#1F2A44"}

	spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
)

func (m *model) bodyHeight() int {
	h := m.height - 2
	if h < 1 {
		h = 1
	}
	return h
}

// listHeight 是右栏列表可见的行数（去掉标题行与子榜单条）。
func (m *model) listHeight() int {
	h := m.bodyHeight() - m.rightHead() - m.previewHeight()
	if h < 1 {
		h = 1
	}
	return h
}

// rightHead 是右栏列表上方的行数：标题行，以及有子榜单时的子榜单条。
func (m *model) rightHead() int {
	if m.hasStrip() {
		return 2
	}
	return 1
}

func (m *model) hasStrip() bool { return len(m.curSubs()) > 0 }

// 最小可用尺寸：40 列能放下单栏的标题、子榜单条与底部提示，10 行能看到几条榜单。
const (
	minWidth  = 40
	minHeight = 10
)

const tooSmallText = "窗口太小，至少需要 40×10"

func (m *model) tooSmall() bool { return m.width < minWidth || m.height < minHeight }

// tooSmallView 在窗口小于最小尺寸时显示一行居中的提示，其余行留空。
func (m *model) tooSmallView() string {
	text := runewidth.Truncate(tooSmallText, m.width, "")
	line := spaces((m.width-runewidth.StringWidth(text))/2) + text
	lines := make([]string, m.height)
	lines[(m.height-1)/2] = line
	return strings.Join(lines, "\n")
}

func (m *model) wide() bool { return m.width >= wideMinWidth }

// leftWidth 按当前菜单最长的来源名自适应，14～18 列。
func (m *model) leftWidth() int {
	w := 0
	for _, s := range m.curSources() {
		if x := runewidth.StringWidth(m.sourceLabel(s)); x > w {
			w = x
		}
	}
	w += 3
	if w < 14 {
		w = 14
	}
	if w > 18 {
		w = 18
	}
	return w
}

func (m *model) sourceLabel(s source) string {
	if s.monitoring() {
		if n := m.monitorUnreadAll(); n > 0 {
			return s.label + " " + itoa(n)
		}
	}
	return s.label
}

// rightX 是右栏第一列的横坐标。
func (m *model) rightX() int {
	if !m.wide() {
		return 0
	}
	return m.leftWidth() + 1
}

func (m *model) rightWidth() int { return m.width - m.rightX() }

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
	if m.tooSmall() {
		return m.tooSmallView()
	}
	var b strings.Builder
	b.WriteString(m.headerLine())
	b.WriteByte('\n')
	var body []string
	if m.det != nil {
		body = m.detailLines(m.width, m.bodyHeight())
	} else {
		body = m.bodyLines()
	}
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

// span 是一行中可点击的一段，x0 ≤ x < x1。
type span struct{ idx, x0, x1 int }

// seg 是横向条（菜单栏、子榜单条）中的一项。
type seg struct {
	span
	text string
}

// strip 把若干标签排成一行，放不下时以 cur 为中心截断，两端显示 ‹ ›。
// 每项左右各留 1 列空白；x0 是第一列的横坐标。
func strip(labels []string, cur, x0, avail int) (segs []seg, left, right bool) {
	if len(labels) == 0 || avail <= 0 {
		return nil, false, false
	}
	texts := make([]string, len(labels))
	ws := make([]int, len(labels))
	for i, l := range labels {
		texts[i] = " " + l + " "
		ws[i] = runewidth.StringWidth(texts[i])
	}
	cur = clampIdx(cur, len(labels))
	lo, hi := window(ws, cur, avail)
	left, right = lo > 0, hi < len(labels)-1
	room := avail
	if left {
		room--
	}
	if right {
		room--
	}
	if ws[cur] > room {
		// 当前项自己都放不下：只显示截断后的当前项。
		lo, hi = cur, cur
		texts[cur] = pad(" "+runewidth.Truncate(labels[cur], room-2, "…")+" ", room)
		ws[cur] = runewidth.StringWidth(texts[cur])
	}
	x := x0
	if left {
		x++
	}
	for i := lo; i <= hi; i++ {
		segs = append(segs, seg{span{i, x, x + ws[i]}, texts[i]})
		x += ws[i]
	}
	return segs, left, right
}

func (m *model) menuLabels() []string {
	out := make([]string, len(m.menus))
	for i, mn := range m.menus {
		out[i] = mn.Name
		if i <= 9 {
			out[i] = itoa(int64(i)) + " " + mn.Name
		}
	}
	return out
}

const brandText = " 今日热榜 "

// headerLayout 计算标题栏：菜单栏各项的位置与右侧的更新时间。
func (m *model) headerLayout() (segs []seg, left, right bool, info string) {
	bw := runewidth.StringWidth(brandText)
	x0 := bw + 1
	labels := m.menuLabels()
	full := 0
	for _, l := range labels {
		full += runewidth.StringWidth(l) + 2
	}
	var infos []string
	switch {
	case m.loading:
		infos = []string{m.spinner() + " 加载中", m.spinner()}
	case !m.updatedAt.IsZero():
		infos = []string{"更新于 " + m.updatedAt.Format("15:04"), m.updatedAt.Format("15:04")}
	}
	infos = append(infos, "")
	// 右侧信息从详细到简略依次尝试，优先让菜单栏完整显示；
	// 都放不下时保留最简略的信息，菜单栏截断。
	info = infos[len(infos)-1]
	for _, s := range infos {
		if x0+full+2+runewidth.StringWidth(s)+1 <= m.width {
			info = s
			break
		}
	}
	if x0+full+2+runewidth.StringWidth(info)+1 > m.width && len(infos) > 1 {
		short := infos[len(infos)-2]
		if m.width-x0-runewidth.StringWidth(short)-3 >= 20 {
			info = short
		}
	}
	avail := m.width - x0 - 1
	if info != "" {
		avail -= runewidth.StringWidth(info) + 2
	}
	segs, left, right = strip(labels, m.menuIdx, x0, avail)
	return segs, left, right, info
}

func (m *model) menuSpans() []span {
	segs, _, _, _ := m.headerLayout()
	out := make([]span, len(segs))
	for i, s := range segs {
		out[i] = s.span
	}
	return out
}

func (m *model) headerLine() string {
	brand := styleBrand.Render(brandText)
	segs, left, right, info := m.headerLayout()
	var b strings.Builder
	b.WriteString(brand)
	b.WriteString(barText(styleBar, " "))
	if left {
		b.WriteString(barText(styleDim, "‹"))
	}
	for _, s := range segs {
		if s.idx == m.menuIdx {
			b.WriteString(styleMenuOn.Render(s.text))
		} else {
			b.WriteString(barText(styleMenu, s.text))
		}
	}
	if right {
		b.WriteString(barText(styleDim, "›"))
	}
	line := b.String()
	infoStyle := styleDim
	if m.loading {
		infoStyle = styleHeader
	}
	gap := m.width - lipgloss.Width(line) - runewidth.StringWidth(info) - 1
	if gap < 0 {
		gap = 0
	}
	line += barText(styleBar, spaces(gap)) + barText(infoStyle, info) + barText(styleBar, " ")
	if w := lipgloss.Width(line); w < m.width {
		line += barText(styleBar, spaces(m.width-w))
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
			return m.sourceLines(m.width, h)
		}
		return m.rightLines(m.width, h)
	}
	lw := m.leftWidth()
	left := m.sourceLines(lw, h)
	right := m.rightLines(m.width-lw-1, h)
	sep := styleSepColor.Render("│")
	out := make([]string, max(h, 0))
	for i := range out {
		out[i] = left[i] + sep + right[i]
	}
	return out
}

// marker 是行首的选中标记：焦点栏为蓝色竖条，非焦点栏为灰色竖条。
func marker(selected, focused bool) string {
	switch {
	case selected && focused:
		return styleMarker.Render("▌")
	case selected:
		return styleDim.Render("▌")
	}
	return " "
}

// sourceLines 渲染左栏：当前菜单的来源，平铺无层级。
func (m *model) sourceLines(w, h int) []string {
	mn, hasMenu := m.curMenu()
	switch {
	case len(m.menus) == 0 && m.menuErr != "":
		return centered([]string{styleError.Render("✗ 菜单加载失败"), "", keyHint("r", "重试")}, w, h)
	case !hasMenu:
		return centered([]string{styleHeader.Render(m.spinner()) + styleDim.Render(" 正在加载…")}, w, h)
	case m.menuErrs[mn.ID] != "":
		return centered([]string{styleError.Render("✗ 加载失败"), "", keyHint("r", "重试")}, w, h)
	case m.data[mn.ID] == nil:
		return centered([]string{styleHeader.Render(m.spinner()) + styleDim.Render(" 正在加载…")}, w, h)
	}
	srcs := m.curSources()
	focused := m.focus == focusTree
	out := make([]string, 0, max(h, 0))
	for i := m.srcOff; i < len(srcs) && len(out) < h; i++ {
		s := srcs[i]
		label := runewidth.Truncate(m.sourceLabel(s), w-3, "…")
		gap := spaces(w - 2 - runewidth.StringWidth(label))
		sel := i == m.srcCur
		var text string
		switch {
		case sel && focused:
			text = styleSel.Render(" " + label + gap)
		case sel:
			text = styleSelBlur.Render(" "+label) + gap
		case m.hovered(focusTree, i):
			text = " " + styleHoverText.Render(label) + gap
		default:
			text = " " + label + gap
		}
		out = append(out, marker(sel, focused)+text)
	}
	return fill(out, w, h)
}

// rightLines 渲染右栏：标题行、子榜单条（有时）与列表。
func (m *model) rightLines(w, h int) []string {
	out := []string{m.titleRow(w)}
	if m.hasStrip() {
		out = append(out, m.stripRow(w))
	}
	pv := m.previewLines(w)
	out = append(out, m.listLines(w, h-len(out)-len(pv))...)
	return append(out, pv...)
}

func (m *model) titleRow(w int) string {
	title := ""
	switch {
	case m.searchQ != "":
		title = "搜索“" + m.searchQ + "”"
	default:
		if s := m.curSource(); s.kind != srcNone {
			title = s.label
			if s.monitoring() && m.mon.unreadOnly {
				title += " · 只看未读"
			}
		} else if mn, ok := m.curMenu(); ok && mn.ID == followMenuID {
			title = mn.Name
		}
	}
	count := ""
	if n := len(m.items); n > 0 && !m.loading {
		count = itoa(int64(n)) + " 条"
		if m.inMonitor() && m.mon.hasMore {
			count = itoa(int64(n)) + "+ 条"
		}
	}
	cw := runewidth.StringWidth(count)
	room := w - 2 - cw - 2
	if room < 4 {
		count, cw, room = "", 0, w-2
	}
	title = runewidth.Truncate(title, room, "…")
	gap := w - 1 - runewidth.StringWidth(title) - cw - 1
	if gap < 0 {
		gap = 0
	}
	return " " + styleCrumb.Render(title) + spaces(gap) + styleDim.Render(count) + " "
}

func (m *model) stripSegs(x0, w int) ([]seg, bool, bool) {
	subs := m.curSubs()
	labels := make([]string, len(subs))
	for i, s := range subs {
		labels[i] = s.label
		if s.note != "" {
			labels[i] += " " + s.note
		}
	}
	return strip(labels, m.subIdx, x0+1, w-2)
}

func (m *model) stripSpans(x0 int) []span {
	segs, _, _ := m.stripSegs(x0, m.rightWidth())
	out := make([]span, len(segs))
	for i, s := range segs {
		out[i] = s.span
	}
	return out
}

func (m *model) stripRow(w int) string {
	segs, left, right := m.stripSegs(0, w)
	var b strings.Builder
	b.WriteString(" ")
	if left {
		b.WriteString(styleDim.Render("‹"))
	}
	subs := m.curSubs()
	for _, s := range segs {
		st := styleDim
		if s.idx == clampIdx(m.subIdx, len(subs)) {
			st = styleTabOn
		}
		// 附注（如“已暂停”）始终灰色显示。
		note := ""
		if s.idx < len(subs) {
			note = subs[s.idx].note
		}
		if i := strings.LastIndex(s.text, " "+note+" "); note != "" && i >= 0 {
			b.WriteString(st.Render(s.text[:i+1]))
			b.WriteString(st.Foreground(colorDim).Bold(false).Render(note))
			b.WriteString(st.Render(s.text[i+1+len(note):]))
			continue
		}
		b.WriteString(st.Render(s.text))
	}
	if right {
		b.WriteString(styleDim.Render("›"))
	}
	line := b.String()
	if lw := lipgloss.Width(line); lw < w {
		line += spaces(w - lw)
	}
	return line
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
	case m.searchQ == "" && m.inFollowMenu() && (!m.loggedIn || m.expired):
		return []string{styleDim.Render("登录后查看订阅：") + styleKey.Render("rebang auth login")}
	case m.searchQ == "" && m.inFollowMenu() && m.data[followMenuID] != nil && len(m.curSources()) == 0:
		return []string{styleDim.Render("还没有订阅任何榜单"), "", styleDim.Render("可在今日热榜网站上添加订阅")}
	case m.listNotice != "":
		lines := []string{styleDim.Render(m.listNotice)}
		if m.searchQ != "" {
			lines = append(lines, "", keyHint("/", "换个关键词")+"   "+keyHint("esc", "返回榜单"))
		}
		return lines
	case m.target.key() != "||" || m.searchQ != "":
		return []string{styleDim.Render("暂无内容"), "", keyHint("↓", "换一个榜单") + "   " + keyHint("r", "刷新")}
	case m.menuErr != "":
		return []string{styleDim.Render(m.menuErr)}
	}
	if mn, ok := m.curMenu(); ok && m.menuErrs[mn.ID] != "" {
		return []string{styleDim.Render(m.menuErrs[mn.ID])}
	}
	if m.busy() {
		return []string{styleHeader.Render(m.spinner()) + " 正在加载…"}
	}
	return []string{styleDim.Render("在左侧选择一个榜单"), "", keyHint("↑↓", "移动")}
}

// centered 把几行文字放到区域中间。
func centered(lines []string, w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	out := make([]string, 0, max(h, 0))
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
		out = append(out, spaces((w-lw)/2)+l)
	}
	return fill(out, w, h)
}

func (m *model) listLines(w, h int) []string {
	if len(m.items) == 0 {
		return centered(m.emptyState(w), w, h)
	}
	if m.inMonitor() {
		return m.monitorLines(w, h)
	}
	out := make([]string, 0, max(h, 0))
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
	// 单栏时优先显示标题：本页有标题因热度列被截断时，整列隐藏热度。
	if !m.wide() && metaCol > 0 {
		withMeta := w - 1 - rankW - 2 - metaCol - 2
		for i := m.listOff; i < len(m.items) && i < m.listOff+h; i++ {
			if runewidth.StringWidth(m.items[i].title) > withMeta {
				metaCol = 0
				break
			}
		}
	}
	for i := m.listOff; i < len(m.items) && len(out) < h; i++ {
		it := m.items[i]
		rank := spaces(rankW-len(itoa(int64(i+1)))) + itoa(int64(i+1))
		metaW := metaCol
		meta := ""
		if metaW > 0 {
			meta = runewidth.Truncate(it.meta, metaW, "…")
			meta = spaces(metaW-runewidth.StringWidth(meta)) + meta
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
		// 排名与标题之间的第二个空格：有 AI 总结时显示标记，不改变列宽。
		mark, hasMark := " ", m.hasInsight(it)
		if hasMark {
			mark = insightMark
		}
		var line string
		if sel && focused {
			rest := title
			if metaW > 0 {
				rest += " " + meta
			}
			rest = pad(rest, w-1-rankW-2)
			if hasMark {
				line = styleSel.Render(rank+" ") + styleSel.Foreground(colorAccent).Render(mark) + styleSel.Render(rest)
			} else {
				line = styleSel.Render(rank + "  " + rest)
			}
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
			if hasMark {
				mark = styleMarker.Render(mark)
			}
			line = rs.Render(rank) + " " + mark + ts.Render(short) + spaces(titleW-runewidth.StringWidth(short))
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
				line += spaces(w - 1 - used)
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
	if m.det != nil {
		return detailHints()
	}
	sub := m.hasStrip()
	var hs []hint
	switch {
	case m.searchQ != "":
		hs = []hint{{"↵", "打开"}, {"空格", "详情"}, {"esc", "返回榜单"}, {"y", "复制"}, {"/", "搜索"}}
		if !m.wide() {
			hs = append(hs[:3], append([]hint{{"tab", "来源"}}, hs[3:]...)...)
		} else {
			hs = append(hs, hint{"p", "预览"})
		}
	case m.picking():
		hs = []hint{{"↑↓", "选择"}, {"↵", "切换"}, {"esc", "返回榜单"}, {m.menuKeys(), "分类"}}
	case !m.wide():
		hs = []hint{{"tab", "来源"}, {"↵", "打开"}, {"空格", "详情"}}
		if m.inMonitor() {
			hs = append(hs, m.unreadHint())
		}
		if sub {
			hs = append(hs, hint{"←→", "切换"})
		}
		hs = append(hs, hint{m.menuKeys(), "分类"}, hint{"y", "复制"}, hint{"/", "搜索"})
	case m.focus == focusTree:
		hs = []hint{{"↑↓", "榜单"}}
		if sub {
			hs = append(hs, hint{"←→", "切换"})
		}
		hs = append(hs, hint{"↵", "进入列表"}, hint{m.menuKeys(), "分类"}, hint{"空格", "详情"}, hint{"/", "搜索"}, hint{"p", "预览"})
	default:
		hs = []hint{{"↵", "打开"}, {"空格", "详情"}}
		if m.inMonitor() {
			hs = append(hs, m.unreadHint())
		}
		if sub {
			hs = append(hs, hint{"←→", "切换"})
		}
		hs = append(hs, hint{"esc", "返回"}, hint{"y", "复制"}, hint{"/", "搜索"}, hint{"J/K", "换来源"}, hint{"p", "预览"})
	}
	return append(hs, hint{"?", "帮助"}, hint{"q", "退出"})
}

// inFollowMenu 表示当前在“订阅”菜单。
func (m *model) inFollowMenu() bool {
	mn, ok := m.curMenu()
	return ok && mn.ID == followMenuID
}

// menuKeys 返回切换菜单的数字键范围，例如“0-8”。
func (m *model) menuKeys() string {
	n := len(m.menus) - 1
	if n > 9 {
		n = 9
	}
	if n <= 0 {
		return "[ ]"
	}
	return "0-" + itoa(int64(n))
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
	if !m.wide() && len(hs) > 0 {
		// 单栏时必须留出第一个提示（tab 来源），账号信息让位。
		_, w1 := render(hs[:1])
		if room := m.width - w1 - 6; lw > room && room > 0 {
			left = runewidth.Truncate(left, room, "…")
			lw = runewidth.StringWidth(left)
		}
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
	sep := ""
	if len(hs) > 0 {
		sep = "  "
	}
	line := barText(styleBar, " ") + barText(leftStyle, left) + barText(styleBar, "  ") + barText(midStyle, mid) + barText(styleBar, spaces(gap)+sep) + hintStr
	if w := lipgloss.Width(line); w < m.width {
		line += barText(styleBar, spaces(m.width-w))
	}
	return line
}

var helpKeys = []struct {
	group string
	keys  []hint
}{
	{"浏览", []hint{{"↑ ↓  j k", "在当前栏上下移动，左栏停下后自动加载"}, {"← →  h l", "切换子榜单"}, {"J K  ⇧↑ ⇧↓", "在榜单中切换上一个、下一个来源"}, {"0-9  [ ]", "切换分类，0 为订阅"}, {"tab", "切换左右栏；单栏时打开来源列表"}, {"g G  PgUp PgDn", "开头、结尾、翻页"}}},
	{"操作", []hint{{"回车", "左栏进入列表；列表中在浏览器打开；来源列表中切换来源"}, {"空格", "详情与 AI 总结（" + insightMark + " 表示有总结）"}, {"p", "打开或关闭底部预览（双栏且至少 24 行时显示）"}, {"u", "内容监控中切换全部、只看未读"}, {"esc", "列表返回左栏；来源列表返回榜单；搜索时返回榜单"}, {"/  y  r", "搜索、复制链接、刷新"}, {"?  q", "帮助、退出"}}},
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
	lines = append(lines, styleDim.Render("鼠标：点击分类、榜单、子榜单切换；双击条目打开；滚轮滚动"), "", styleDim.Render("按任意键返回"))
	// 窗口不够高时去掉空行。
	if len(lines) > h {
		compact := lines[:0:0]
		for _, l := range lines {
			if l != "" {
				compact = append(compact, l)
			}
		}
		lines = compact
		if len(lines) > h {
			// 仍然放不下时去掉标题“快捷键”。
			lines = lines[1:]
		}
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
	out := make([]string, 0, max(h, 0))
	for i := 0; i < top; i++ {
		out = append(out, "")
	}
	for _, l := range lines {
		if lipgloss.Width(l)+left > w {
			l = runewidth.Truncate(l, w-left, "…")
		}
		out = append(out, spaces(left)+l)
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
	return s + spaces(w-sw)
}

// spaces 返回 n 个空格，n ≤ 0 时返回空串（窗口很小时宽度可能算成负数）。
func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

// fill 补齐到 h 行，每行 w 列。
func fill(lines []string, w, h int) []string {
	if h <= 0 {
		return nil
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	blank := spaces(w)
	for len(lines) < h {
		lines = append(lines, blank)
	}
	for i, l := range lines {
		if lipgloss.Width(l) < w {
			lines[i] = l + spaces(w-lipgloss.Width(l))
		}
	}
	return lines
}

// unreadHint 是内容监控中切换“全部 / 只看未读”的提示。
func (m *model) unreadHint() hint {
	if m.mon.unreadOnly {
		return hint{"u", "看全部"}
	}
	return hint{"u", "只看未读"}
}
