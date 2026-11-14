package tui

import (
	"errors"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/summary"
	"github.com/rebang-today/rebang-cli/internal/ui"
)

// AI 总结（D-26）：预览区显示完整总结。
//
// 列表加载后批量查询本页条目是否有总结，结果按 item_key 缓存；光标在有总结的条目上停下约 200ms
// 后才请求完整总结（按 item_key 缓存），请求期间先显示一句话总结。

// errNoSummary 表示条目暂无 AI 总结（服务端 404）。
var errNoSummary = errors.New("暂无 AI 总结")

const (
	// previewMinHeight 是在榜单下方显示预览所需的最小终端高度。
	previewMinHeight = 24
	// previewMinRows 是下方预览区最少的行数（含分隔线）。
	previewMinRows = 6
	// insightWait 是光标停下后到请求完整总结之间的等待时间。
	insightWait = 200 * time.Millisecond
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
	// insightWaitMsg 是光标停下后请求完整总结的定时，seq 过期时丢弃。
	insightWaitMsg struct {
		seq int
		key string
	}
)

// insightEntry 是按 item_key 缓存的完整总结。
type insightEntry struct {
	loading bool
	in      *api.Insight
	err     error
}

// previewLoading 表示预览正在加载完整总结，需要转圈动画。
func (m *model) previewLoading() bool {
	it, ok := m.selected()
	if !ok || !m.previewShown() {
		return false
	}
	e := m.insights[it.key]
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

// ---- 预览 ----

// previewHeight 是双栏时榜单下方预览区占用的行数（含分隔线），不显示时为 0。
func (m *model) previewHeight() int {
	if m.previewOff || !m.wide() || m.height < previewMinHeight || m.help || len(m.items) == 0 {
		return 0
	}
	return max(previewMinRows, m.bodyHeight()*2/5)
}

// previewFull 表示单栏时显示整栏的预览（p 切换）。
func (m *model) previewFull() bool { return m.pvFull && !m.wide() && m.focus == focusList && !m.help }

// previewShown 表示当前能看到预览。
func (m *model) previewShown() bool {
	return m.previewHeight() > 0 || m.previewFull()
}

// previewBox 返回预览正文的宽度与可见行数。
func (m *model) previewBox() (w, h int) {
	switch {
	case m.previewFull():
		return m.width - 2, m.bodyHeight() - 1
	case m.previewHeight() > 0:
		return m.rightWidth() - 2, m.previewHeight() - 1
	}
	return 0, 0
}

// togglePreview：单栏时在榜单与整栏预览之间切换；双栏时开关预览区并保存。
func (m *model) togglePreview() {
	if !m.wide() {
		m.pvFull = !m.pvFull
		m.focus, m.pvOff = focusList, 0
		m.clamp()
		return
	}
	m.previewOff = !m.previewOff
	switch {
	case m.previewOff:
		m.flash("已关闭预览", false)
	case m.height < previewMinHeight:
		m.flash("已打开预览（窗口至少 24 行时显示在榜单下方）", false)
	default:
		m.flash("已打开预览", false)
	}
	m.clamp()
}

// pagePreview 把预览向下翻一屏，翻到底后回到开头；内容放得下时不做任何事。
func (m *model) pagePreview() {
	w, h := m.previewBox()
	if h <= 0 {
		return
	}
	n := len(m.previewContent(w))
	if n <= h {
		return
	}
	page := h - 1 // 最后一行留给“↓ 更多”
	if m.pvOff+page >= n {
		m.pvOff = 0
		return
	}
	m.pvOff = min(m.pvOff+page, n-page)
}

// previewOverflows 表示当前预览内容放不下，需要翻页。
func (m *model) previewOverflows() bool {
	w, h := m.previewBox()
	return h > 0 && len(m.previewContent(w)) > h
}

// syncPreview 在选中条目变化时让预览回到顶部，并在光标停下约 200ms 后请求完整总结。
func (m *model) syncPreview() tea.Cmd {
	it, _ := m.selected()
	if it.key != m.pvKey {
		m.pvKey, m.pvOff, m.pvWait = it.key, 0, ""
		m.insSeq++
	}
	k := it.key
	if k == "" || m.pvWait == k || !m.canInsight() || !m.previewShown() {
		return nil
	}
	if _, has := m.shorts[k]; !has || m.insights[k] != nil {
		return nil
	}
	m.pvWait = k
	seq := m.insSeq
	return tea.Tick(insightWait, func(time.Time) tea.Msg { return insightWaitMsg{seq, k} })
}

// loadInsight 在定时到达且光标仍停在该条目上时请求完整总结。
func (m *model) loadInsight(msg insightWaitMsg) tea.Cmd {
	if msg.seq != m.insSeq || m.pvKey != msg.key || m.insights[msg.key] != nil || !m.canInsight() {
		return nil
	}
	m.insights[msg.key] = &insightEntry{loading: true}
	src, key := m.opt.Source, msg.key
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
		if s := ui.CleanText(msg.in.SummaryShort); s != "" {
			m.shorts[msg.key] = s
		}
	}
}

// metaText 返回条目的“来源 · 时间 · 热度”（内容监控为“来源 · 命中时间”）。
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

// previewContent 返回预览的全部正文行（宽 w，未截取翻页窗口）：元信息、命中，
// “AI 总结”（一句话总结与各分区），“简介”（描述）；都没有时为“暂无摘要”。
func (m *model) previewContent(w int) []string {
	it, ok := m.selected()
	if !ok || w <= 0 {
		return nil
	}
	st := insightStyle()
	var out []string
	add := func(lines ...summary.Line) { out = append(out, summary.Render(lines, w, st)...) }
	if m.previewFull() {
		// 整栏预览看不到榜单，先显示标题。
		for _, l := range summary.Plain([]summary.Line{{Kind: summary.Para, Text: it.title}}, w) {
			out = append(out, styleCrumb.Render(l))
		}
	}
	if meta := metaText(it); meta != "" {
		out = append(out, styleDim.Render(runewidth.Truncate(meta, w, "…")))
	}
	if mt := matchText(it); mt != "" {
		add(summary.Line{Kind: summary.Para, Label: "命中：", Text: mt})
	}
	short, has := m.shorts[it.key]
	if has && short != "" {
		out = append(out, "")
		add(summary.Line{Kind: summary.Heading, Text: "AI 总结"}, summary.Line{Kind: summary.Para, Text: short})
		switch e := m.insights[it.key]; {
		case e == nil || e.loading:
			if m.canInsight() {
				out = append(out, "", styleDim.Render(m.spinner()+" 加载中…"))
			}
		case e.in != nil:
			if sec := summary.Sections(e.in.ParseDetail()); len(sec) > 0 {
				out = append(out, "")
				add(sec...)
			}
		}
		// 出错时只显示一句话总结，不显示错误。
	}
	desc := it.desc
	if placeholderOnly(desc) {
		desc = ""
	}
	if desc != "" {
		out = append(out, "")
		add(summary.Line{Kind: summary.Heading, Text: "简介"}, summary.Line{Kind: summary.Para, Text: desc})
	}
	if (!has || short == "") && desc == "" {
		out = append(out, "", styleDim.Render("暂无摘要"))
	}
	return out
}

// placeholderPat 匹配“[图片]”“[视频]”这类方括号里的短占位文字。
var placeholderPat = regexp.MustCompile(`\[[^\[\]]{1,6}\]|【[^【】]{1,6}】`)

// placeholderOnly 表示文本去掉占位文字与空白后什么都不剩（如简介只有“[图片]”）。
func placeholderOnly(s string) bool {
	return strings.TrimSpace(placeholderPat.ReplaceAllString(s, "")) == ""
}

// previewBody 把预览正文按翻页位置截成 h 行（每行左边留 1 列），超出时右下角显示“↓ 更多”。
func (m *model) previewBody(w, h int) []string {
	if w <= 0 || h <= 0 {
		return fill(nil, max(w, 0), h)
	}
	all := m.previewContent(w - 2)
	page := h
	if len(all) > h {
		page = h - 1 // 放不下时最后一行专门放“↓ 更多”，不盖住正文
	}
	off := clampOff(m.pvOff, len(all), page)
	end := min(off+page, len(all))
	out := make([]string, 0, h)
	for _, l := range all[off:end] {
		out = append(out, " "+l)
	}
	out = fill(out, w, h)
	if end < len(all) {
		tag := styleDim.Render("↓ 更多")
		out[h-1] = spaces(w-lipgloss.Width(tag)-1) + tag + " "
		if lipgloss.Width(out[h-1]) > w {
			out[h-1] = ansiTruncate(tag, w)
		}
	}
	return out
}

// previewLines 渲染榜单下方的预览区：分隔线与预览正文。
func (m *model) previewLines(w int) []string {
	h := m.previewHeight()
	if h == 0 || w <= 0 {
		return nil
	}
	return append([]string{styleSepColor.Render(strings.Repeat("─", w))}, m.previewBody(w, h-1)...)
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
