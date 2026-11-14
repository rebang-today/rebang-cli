package tui

import (
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/summary"
	"github.com/rebang-today/rebang-cli/internal/ui"
)

// AI 总结（D-26）：列表标记、双栏底部预览与全屏详情页。
//
// 列表加载后批量查询本页条目是否有总结（只在登录时），结果按 item_key 缓存；
// 移动光标不发请求，预览只用批量结果与列表数据；详情页打开时才查询完整总结。

// errNoSummary 表示条目暂无 AI 总结（服务端 404）。
var errNoSummary = errors.New("暂无 AI 总结")

const (
	// insightMark 是有 AI 总结的条目标题前的标记，在东亚宽度模式下也只占 1 列。
	insightMark = "✦"
	// previewMinHeight 是显示预览所需的最小终端高度。
	previewMinHeight = 24
	// previewRows 是预览内容的行数（不含分隔线）。
	previewRows = 6
)

type (
	shortsMsg struct {
		keys []string
		res  map[string]string
		err  error
	}
	insightMsg struct {
		key string
		in  *api.Insight
		err error
	}
)

// insightEntry 是详情页按 item_key 缓存的完整总结。
type insightEntry struct {
	loading bool
	in      *api.Insight
	err     error
}

// detailState 是打开的详情页：打开时的条目与滚动位置。
type detailState struct {
	it  row
	off int
}

// detailLoading 表示详情页正在加载完整总结，需要转圈动画。
func (m *model) detailLoading() bool {
	if m.det == nil {
		return false
	}
	e := m.insights[m.det.it.key]
	return e != nil && e.loading
}

// canInsight 表示可以查询 AI 总结。接口不要求登录；登录失效（服务端返回 401）后不再请求，
// 避免带着失效的令牌反复失败。
func (m *model) canInsight() bool { return !m.expired }

// loadShorts 批量查询本页还没查过的 item_key。
func (m *model) loadShorts() tea.Cmd {
	if !m.canInsight() {
		return nil
	}
	var keys []string
	seen := map[string]bool{}
	for _, it := range m.items {
		if it.key != "" && !m.shortSeen[it.key] && !seen[it.key] {
			seen[it.key] = true
			keys = append(keys, it.key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	src := m.opt.Source
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		res, err := src.InsightShorts(c, keys)
		return shortsMsg{keys, res, err}
	}
}

// onShorts 合并批量结果；失败时静默，下次刷新再查。
func (m *model) onShorts(msg shortsMsg) {
	if msg.err != nil {
		if isAuth(msg.err) {
			m.markExpired()
		}
		return
	}
	for _, k := range msg.keys {
		m.shortSeen[k] = true
	}
	for k, v := range msg.res {
		m.shorts[k] = ui.CleanText(v)
	}
}

// hasInsight 表示条目有 AI 总结（批量查询结果或已加载的详情）。
func (m *model) hasInsight(it row) bool {
	if it.key == "" {
		return false
	}
	if _, ok := m.shorts[it.key]; ok {
		return true
	}
	e := m.insights[it.key]
	return e != nil && e.in != nil
}

// ---- 预览 ----

// previewHeight 是双栏右栏底部预览占用的行数（含分隔线），不显示时为 0。
func (m *model) previewHeight() int {
	if m.previewOff || !m.wide() || m.height < previewMinHeight || m.help || len(m.items) == 0 {
		return 0
	}
	return previewRows + 1
}

// togglePreview 切换预览并提示；单栏或高度不够时说明原因。
func (m *model) togglePreview() {
	m.previewOff = !m.previewOff
	switch {
	case m.previewOff:
		m.flash("已关闭预览", false)
	case !m.wide() || m.height < previewMinHeight:
		m.flash("已打开预览（窗口放大到双栏且至少 24 行时显示）", false)
	default:
		m.flash("已打开预览", false)
	}
	m.clamp()
}

// metaText 返回条目的“来源 · 时间 · 热度”。
func metaText(it row) string {
	var parts []string
	for _, s := range []string{it.src, it.when, it.heat} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " · ")
}

var (
	styleInsightHead = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleQuote       = lipgloss.NewStyle().Foreground(colorQuote)
	colorQuote       = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8B93A7"}
	styleBold        = lipgloss.NewStyle().Bold(true)
)

func insightStyle() summary.Style {
	return summary.Style{
		Heading: func(s string) string { return styleInsightHead.Render(s) },
		Label:   func(s string) string { return styleBold.Render(s) },
		Bold:    func(s string) string { return styleBold.Render(s) },
		Quote:   func(s string) string { return styleQuote.Render(s) },
		Dim:     func(s string) string { return styleDim.Render(s) },
	}
}

// previewLines 渲染预览：分隔线、元信息，再是 AI 一句话总结、描述或“暂无摘要”。
func (m *model) previewLines(w int) []string {
	h := m.previewHeight()
	if h == 0 || w <= 0 {
		return nil
	}
	out := []string{styleSepColor.Render(strings.Repeat("─", w))}
	it, ok := m.selected()
	if !ok {
		return fill(out, w, h)
	}
	inner := w - 2
	var body []string
	if meta := metaText(it); meta != "" {
		body = append(body, styleDim.Render(runewidth.Truncate(meta, inner, "…")))
	}
	lines := []summary.Line{}
	switch short, has := m.shorts[it.key]; {
	case has && short != "":
		lines = append(lines, summary.Line{Kind: summary.Para, Label: "AI 总结", Text: short})
	case it.desc != "":
		lines = append(lines, summary.Line{Kind: summary.Para, Text: it.desc})
	default:
		lines = append(lines, summary.Line{Kind: summary.Byline, Text: "暂无摘要"})
	}
	st := insightStyle()
	st.Label = func(s string) string { return styleInsightHead.Render(s) }
	text := summary.Render(lines, inner, st)
	room := previewRows - len(body)
	if len(text) > room {
		text = text[:max(room, 0)]
		if n := len(text); n > 0 {
			// 最后一行放不下：截断并加省略号。
			last := ansiTruncate(text[n-1], inner-1)
			text[n-1] = last + styleDim.Render("…")
		}
	}
	body = append(body, text...)
	for _, l := range body {
		out = append(out, " "+l)
	}
	return fill(out, w, h)
}

// ansiTruncate 按显示宽度截断带样式的文本。
func ansiTruncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}

// ---- 详情页 ----

// openDetail 打开当前条目的详情页；登录时按需加载完整总结。
func (m *model) openDetail() tea.Cmd {
	it, ok := m.selected()
	if !ok {
		return nil
	}
	m.det = &detailState{it: it}
	m.hoverOK = false
	m.setPointer(false)
	if !m.canInsight() || it.key == "" {
		return nil
	}
	if e := m.insights[it.key]; e != nil && (e.loading || e.in != nil || errors.Is(e.err, errNoSummary)) {
		return nil
	}
	m.insights[it.key] = &insightEntry{loading: true}
	src, key := m.opt.Source, it.key
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		in, err := src.Insight(c, key)
		return insightMsg{key, in, err}
	}
}

func (m *model) onInsight(msg insightMsg) {
	e := &insightEntry{in: msg.in, err: msg.err}
	if msg.err == nil && msg.in == nil {
		e.err = errNoSummary
	}
	m.insights[msg.key] = e
	if msg.err != nil && isAuth(msg.err) {
		m.markExpired()
	}
	if msg.in != nil {
		m.shorts[msg.key] = ui.CleanText(msg.in.SummaryShort)
	}
}

// detailKey 处理详情页打开时的按键。
func (m *model) detailKey(key string) tea.Cmd {
	page := max(m.bodyHeight()-1, 1)
	switch key {
	case "q", "esc", " ", "space":
		m.det = nil
		m.clamp()
		return nil
	case "enter":
		return m.openURL(m.det.it.url)
	case "up", "k":
		m.det.off--
	case "down", "j":
		m.det.off++
	case "pgup", "ctrl+b", "b":
		m.det.off -= page
	case "pgdown", "ctrl+f", "f":
		m.det.off += page
	case "home", "g":
		m.det.off = 0
	case "end", "G":
		m.det.off = 1 << 20
	case "y":
		return m.copyURL(m.det.it.url)
	}
	m.clampDetail()
	return nil
}

func (m *model) clampDetail() {
	if m.det == nil {
		return
	}
	n := len(m.detailContent(m.detailWidth()))
	m.det.off = clampOff(m.det.off, n, m.bodyHeight())
}

// detailWidth 是详情正文的宽度：左右各留 2 列，宽屏时最多 100 列便于阅读。
func (m *model) detailWidth() int {
	return min(m.width-4, 100)
}

// detailContent 返回详情页的全部正文行（未截取滚动窗口）。
func (m *model) detailContent(w int) []string {
	if m.det == nil || w <= 0 {
		return nil
	}
	it := m.det.it
	st := insightStyle()
	var out []string
	add := func(lines ...summary.Line) { out = append(out, summary.Render(lines, w, st)...) }

	out = append(out, "")
	for _, l := range summary.Plain([]summary.Line{{Kind: summary.Para, Text: it.title}}, w) {
		out = append(out, styleCrumb.Render(l))
	}
	if meta := metaText(it); meta != "" {
		add(summary.Line{Kind: summary.Byline, Text: meta})
	}
	if it.desc != "" {
		out = append(out, "")
		add(summary.Line{Kind: summary.Para, Text: it.desc})
	}
	out = append(out, "")
	add(summary.Line{Kind: summary.Heading, Text: "AI 总结"})

	state := func(s string, st lipgloss.Style) {
		for _, l := range summary.Plain([]summary.Line{{Kind: summary.Para, Text: s}}, w) {
			out = append(out, st.Render(l))
		}
	}
	e := m.insights[it.key]
	switch {
	case it.key == "":
		state("暂无 AI 总结", styleDim)
	case e == nil && !m.canInsight():
		state("登录已失效，退出后运行 rebang auth login", styleDim)
	case e != nil && e.err != nil && isAuth(e.err):
		state("登录已失效，退出后运行 rebang auth login", styleDim)
	case e == nil || e.loading:
		state(m.spinner()+" 加载中…", styleDim)
	case errors.Is(e.err, errNoSummary):
		state("暂无 AI 总结", styleDim)
	case e.err != nil:
		state("✗ "+errText(e.err), styleError)
	default:
		if s := ui.CleanText(e.in.SummaryShort); s != "" {
			add(summary.Line{Kind: summary.Para, Text: s})
		}
		if sec := summary.Sections(e.in.ParseDetail()); len(sec) > 0 {
			out = append(out, "")
			add(sec...)
		}
	}
	return out
}

// detailLines 渲染详情页正文区域（h 行，每行 w 列）。
func (m *model) detailLines(w, h int) []string {
	cw := m.detailWidth()
	left := 2
	if w > cw+4 {
		left = (w - cw) / 2
	}
	all := m.detailContent(cw)
	off := clampOff(m.det.off, len(all), h)
	end := min(off+h, len(all))
	out := make([]string, 0, max(h, 0))
	for _, l := range all[off:end] {
		out = append(out, spaces(left)+l)
	}
	out = fill(out, w, h)
	// 还有更多内容时在右下角提示可以滚动。
	if end < len(all) && h > 0 && w >= 4 {
		pct := itoa(int64(end*100/len(all))) + "%"
		tag := styleDim.Render("↓ " + pct)
		tw := lipgloss.Width(tag)
		if room := w - tw - 1; room >= 0 {
			base := ansiTruncate(out[h-1], room)
			out[h-1] = base + spaces(room-lipgloss.Width(base)) + tag + " "
		}
	}
	return out
}

// detailHints 是详情页底部的按键提示。
func detailHints() []hint {
	return []hint{{"↑↓", "滚动"}, {"↵", "打开原文"}, {"esc", "返回"}, {"y", "复制"}, {"q", "关闭"}}
}

// shortTime 把 RFC3339 时间格式化为简短形式：今天只显示时刻，今年显示月日。
func shortTime(s string, now time.Time) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ""
	}
	t = t.In(now.Location())
	switch {
	case t.Year() == now.Year() && t.YearDay() == now.YearDay():
		return t.Format("15:04")
	case t.Year() == now.Year():
		return t.Format("1月2日 15:04")
	}
	return t.Format("2006年1月2日")
}

// listSourceName 返回当前榜单的来源名称，有多个子榜单时附带子榜单名。
func (m *model) listSourceName() string {
	s := m.curSource()
	if s.kind == srcNone {
		return ""
	}
	name := s.label
	if subs := m.curSubs(); len(subs) > 1 {
		name += " · " + subs[clampIdx(m.subIdx, len(subs))].label
	}
	return name
}
