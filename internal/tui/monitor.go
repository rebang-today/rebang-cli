package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/cli"
	"github.com/rebang-today/rebang-cli/internal/summary"
	"github.com/rebang-today/rebang-cli/internal/ui"
)

// 内容监控（D-27）：在“订阅”的“内容监控”来源下只读查看命中内容。
// 子榜单条是“全部”加每个计划；列表按 cursor 分页，光标接近末尾时自动加载下一页。

const (
	// monitorMoreAt 是光标距离列表末尾多少条以内时加载下一页。
	monitorMoreAt = 5
	unreadDot     = "●"
)

// monitorState 是内容监控的数据与分页状态。
type monitorState struct {
	plans  []api.MonitorPlan
	unread *api.MonitorUnread
	// cursor、hasMore 是下一页的位置；more 表示正在加载下一页。
	cursor  string
	hasMore bool
	more    bool
	// unreadOnly 表示只看未读（u 切换，保存到状态文件）。
	unreadOnly bool
	// sub、loadedUnread 是上一次加载时的子榜单与筛选，变化时清空列表重新加载。
	sub          int
	loadedUnread bool
	loaded       bool
}

type (
	monitorMsg struct {
		seq    int
		plans  []api.MonitorPlan
		unread *api.MonitorUnread
		page   *api.MonitorHits
		err    error
	}
	monitorMoreMsg struct {
		seq  int
		page *api.MonitorHits
		err  error
	}
	monitorUnreadMsg struct {
		unread *api.MonitorUnread
		err    error
	}
)

// inMonitor 表示右栏正在显示内容监控（不在搜索结果中）。
func (m *model) inMonitor() bool { return m.searchQ == "" && m.curSource().monitoring() }

// hasMonitorSource 表示当前菜单数据中有内容监控来源。
func (m *model) hasMonitorSource(d *menuData) bool {
	for _, s := range sources(d) {
		if s.monitoring() {
			return true
		}
	}
	return false
}

// monitorWeb 返回网站上管理监控计划的地址。
func (m *model) monitorWeb() string {
	web := strings.TrimRight(m.opt.Web, "/")
	if web == "" {
		web = "https://rebang.today"
	}
	return web + "/monitoring"
}

// loadMonitor 加载计划、未读数与命中列表第一页。reset 为 true 时先清空列表（切换计划、筛选或来源）。
func (m *model) loadMonitor(reset bool) tea.Cmd {
	m.debSeq++
	m.debPending = false
	if !m.loggedIn || m.expired {
		m.clearList()
		return nil
	}
	t, _ := m.curSource().target(0)
	if t.key() != m.target.key() || m.mon.sub != m.subIdx || m.mon.loadedUnread != m.mon.unreadOnly {
		reset = true
	}
	m.listSeq++
	seq := m.listSeq
	if reset {
		m.items, m.listCur, m.listOff = nil, 0, 0
	}
	m.target, m.searchQ, m.loading = t, "", true
	m.access, m.listNotice, m.listErr = nil, "", ""
	m.mon.more, m.mon.sub, m.mon.loadedUnread = false, m.subIdx, m.mon.unreadOnly
	src, sub, unreadOnly := m.opt.Source, m.subIdx, m.mon.unreadOnly
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		plans, err := src.MonitorPlans(c)
		if err != nil {
			return monitorMsg{seq: seq, err: err}
		}
		unread, err := src.MonitorUnread(c)
		if err != nil {
			return monitorMsg{seq: seq, err: err}
		}
		msg := monitorMsg{seq: seq, plans: plans, unread: unread}
		if len(plans) == 0 {
			return msg
		}
		q := api.MonitorHitsQuery{UnreadOnly: unreadOnly, PageSize: api.MonitorHitsPageMax}
		if sub > 0 && sub <= len(plans) {
			q.PlanID = plans[sub-1].ID
		}
		msg.page, msg.err = src.MonitorHits(c, q)
		return msg
	}
}

func (m *model) onMonitor(msg monitorMsg) tea.Cmd {
	if msg.seq != m.listSeq {
		return nil
	}
	m.loading = false
	if msg.err != nil && msg.plans == nil {
		m.items = nil
		m.listErr = errText(msg.err)
		m.fail(msg.err)
		return nil
	}
	m.mon.plans, m.mon.unread, m.mon.loaded = msg.plans, msg.unread, true
	m.updatedAt = m.opt.Now()
	if len(msg.plans) == 0 {
		m.items, m.listCur, m.listOff = nil, 0, 0
		m.mon.cursor, m.mon.hasMore = "", false
		m.listNotice = "还没有监控计划，可在网站创建：" + m.monitorWeb()
		return nil
	}
	if m.subIdx > len(msg.plans) {
		// 计划被删除：回到“全部”。
		m.subIdx, m.mon.sub = 0, 0
		return m.loadMonitor(true)
	}
	if msg.err != nil {
		m.items = nil
		m.listErr = errText(msg.err)
		m.fail(msg.err)
		return nil
	}
	keep := m.selectedURL()
	m.items = m.items[:0]
	m.addHits(msg.page.Items)
	m.mon.cursor, m.mon.hasMore = msg.page.NextCursor, msg.page.HasMore && msg.page.NextCursor != ""
	m.restoreCursor(keep)
	m.setMonitorNotice()
	return tea.Batch(m.loadShorts(), m.maybeMore())
}

func (m *model) setMonitorNotice() {
	m.listNotice = ""
	if len(m.items) == 0 {
		m.listNotice = "还没有命中内容"
		if m.mon.unreadOnly {
			m.listNotice = "没有未读的命中内容（u 查看全部）"
		}
	}
}

// addHits 把命中内容追加到列表，按 item_key 去重。
func (m *model) addHits(hits []api.MonitorHit) {
	seen := make(map[string]bool, len(m.items))
	for _, it := range m.items {
		seen[it.key] = true
	}
	now := m.opt.Now()
	for _, h := range hits {
		if h.ItemKey != "" && seen[h.ItemKey] {
			continue
		}
		seen[h.ItemKey] = true
		var kws []string
		for _, k := range h.MatchedKeywords() {
			kws = append(kws, ui.CleanText(k))
		}
		mobile := h.MobileURL
		if mobile == h.URL() {
			mobile = ""
		}
		m.items = append(m.items, row{
			title: ui.CleanText(h.Title), url: h.URL(), mobile: mobile, desc: ui.CleanText(h.Description),
			key: h.ItemKey, src: ui.CleanText(h.SourceName), when: relTime(h.MatchedAt, now),
			mon: true, unread: !h.IsRead, match: strings.Join(kws, "、"), plans: h.PlanMatches,
		})
	}
}

// maybeMore 在光标接近末尾且还有下一页时加载下一页。
func (m *model) maybeMore() tea.Cmd {
	if !m.inMonitor() || m.loading || m.mon.more || !m.mon.hasMore || m.det != nil {
		return nil
	}
	if len(m.items)-1-m.listCur >= monitorMoreAt {
		return nil
	}
	m.mon.more = true
	seq, src := m.listSeq, m.opt.Source
	q := api.MonitorHitsQuery{Cursor: m.mon.cursor, UnreadOnly: m.mon.unreadOnly, PageSize: api.MonitorHitsPageMax}
	if s := m.mon.sub; s > 0 && s <= len(m.mon.plans) {
		q.PlanID = m.mon.plans[s-1].ID
	}
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		p, err := src.MonitorHits(c, q)
		return monitorMoreMsg{seq, p, err}
	}
}

func (m *model) onMonitorMore(msg monitorMoreMsg) tea.Cmd {
	if msg.seq != m.listSeq {
		return nil
	}
	m.mon.more = false
	if msg.err != nil {
		// 不自动重试，避免反复失败；r 从第一页重新加载。
		m.mon.hasMore = false
		m.fail(msg.err)
		return nil
	}
	m.addHits(msg.page.Items)
	m.mon.cursor, m.mon.hasMore = msg.page.NextCursor, msg.page.HasMore && msg.page.NextCursor != "" && len(msg.page.Items) > 0
	m.clamp()
	return tea.Batch(m.loadShorts(), m.maybeMore())
}

// loadMonitorUnread 只刷新未读数（进入“订阅”时用于左栏的总未读数）。
func (m *model) loadMonitorUnread() tea.Cmd {
	if !m.loggedIn || m.expired {
		return nil
	}
	src := m.opt.Source
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		u, err := src.MonitorUnread(c)
		return monitorUnreadMsg{u, err}
	}
}

func (m *model) onMonitorUnread(msg monitorUnreadMsg) {
	if msg.err != nil {
		if isAuth(msg.err) {
			m.markExpired()
		}
		return
	}
	if msg.unread != nil {
		m.mon.unread = msg.unread
	}
}

// markRead 打开命中内容时在本地标为已读，并在后台通知服务端；失败不影响打开原文。
func (m *model) markRead(it row) tea.Cmd {
	if !it.mon || !it.unread || it.key == "" || !m.loggedIn || m.expired {
		return nil
	}
	for i := range m.items {
		if m.items[i].key == it.key {
			m.items[i].unread = false
		}
	}
	if m.det != nil && m.det.it.key == it.key {
		m.det.it.unread = false
	}
	src, key := m.opt.Source, it.key
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		u, err := src.MonitorRead(c, key)
		return monitorUnreadMsg{u, err}
	}
}

// toggleUnread 切换“全部 / 只看未读”，从第一页重新加载。
func (m *model) toggleUnread() tea.Cmd {
	if !m.inMonitor() {
		return nil
	}
	m.mon.unreadOnly = !m.mon.unreadOnly
	if m.mon.unreadOnly {
		m.flash("只看未读", false)
	} else {
		m.flash("显示全部", false)
	}
	return m.loadMonitor(true)
}

// monitorSubs 返回内容监控的子榜单：“全部”与每个计划，带未读数与状态。
func (m *model) monitorSubs() []subTab {
	if len(m.mon.plans) == 0 {
		return nil
	}
	count := func(n int64) string {
		if n <= 0 {
			return ""
		}
		return " " + itoa(n)
	}
	var all int64
	if m.mon.unread != nil {
		all = m.mon.unread.All
	}
	out := []subTab{{key: "all", label: "全部" + count(all)}}
	for _, p := range m.mon.plans {
		st := subTab{key: itoa(p.ID), label: ui.CleanText(p.Name) + count(m.mon.unread.Plan(p.ID))}
		if p.Status == api.MonitorPaused || p.Status == api.MonitorMembershipExpired {
			st.note = cli.MonitorStatusText(p.Status)
		}
		out = append(out, st)
	}
	return out
}

// monitorUnreadAll 是左栏“内容监控”后显示的总未读数。
func (m *model) monitorUnreadAll() int64 {
	if m.mon.unread == nil || !m.loggedIn || m.expired {
		return 0
	}
	return m.mon.unread.All
}

// relTime 把命中时间（Unix 秒）格式化为相对时间。
func relTime(ts int64, now time.Time) string {
	if ts <= 0 {
		return ""
	}
	t := time.Unix(ts, 0).In(now.Location())
	d := now.Sub(t)
	y, mo, dd := now.Date()
	today := time.Date(y, mo, dd, 0, 0, 0, 0, now.Location())
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return itoa(int64(d/time.Minute)) + " 分钟前"
	case !t.Before(today):
		return itoa(int64(d/time.Hour)) + " 小时前"
	case !t.Before(today.AddDate(0, 0, -1)):
		return t.Format("昨天 15:04")
	case t.Year() == now.Year():
		return t.Format("1月2日 15:04")
	}
	return t.Format("2006年1月2日")
}

// monitorLines 渲染命中列表：未读圆点、AI 总结标记、标题、灰色的“来源 · 命中词”，右侧命中时间。
func (m *model) monitorLines(w, h int) []string {
	out := make([]string, 0, max(h, 0))
	focused := m.focus == focusList
	timeW := 0
	for i := m.listOff; i < len(m.items) && i < m.listOff+h; i++ {
		timeW = max(timeW, runewidth.StringWidth(m.items[i].when))
	}
	for i := m.listOff; i < len(m.items) && len(out) < h; i++ {
		it := m.items[i]
		sel := i == m.listCur
		// 行首标记 1 列 + 圆点 1 列 + 空格 + 总结标记 1 列 + 正文 + 行尾 1 列。
		body := w - 1 - 3 - 1
		tw := timeW
		if body-tw-2 < 8 {
			tw = 0
		}
		textW := body
		if tw > 0 {
			textW = body - tw - 2
		}
		if textW < 1 {
			textW = max(body, 0)
		}
		title := runewidth.Truncate(it.title, textW, "…")
		sub := strings.Join(nonEmpty(it.src, it.match), " · ")
		subText := ""
		if rest := textW - runewidth.StringWidth(title) - 2; sub != "" && rest >= 6 {
			subText = runewidth.Truncate(sub, rest, "…")
		}
		gap := textW - runewidth.StringWidth(title)
		if subText != "" {
			gap -= 2 + runewidth.StringWidth(subText)
		}
		when := ""
		if tw > 0 {
			when = runewidth.Truncate(it.when, tw, "")
			when = spaces(tw-runewidth.StringWidth(when)) + when
		}
		dot, mark := " ", " "
		if it.unread {
			dot = unreadDot
		}
		if m.hasInsight(it) {
			mark = insightMark
		}
		var line string
		if sel && focused {
			st := styleSel
			line = st.Foreground(colorAccent).Render(dot) + st.Render(" ") + st.Foreground(colorAccent).Render(mark) + st.Render(title)
			if subText != "" {
				line += st.Render("  ") + st.Foreground(colorDim).Render(subText)
			}
			line += st.Render(spaces(max(gap, 0)))
			if tw > 0 {
				line += st.Render("  " + when)
			}
			line += st.Render(" ")
		} else {
			ts := lipgloss.NewStyle()
			switch {
			case sel:
				ts = styleSelBlur
			case m.hovered(focusList, i):
				ts = styleHoverText
			}
			line = styleMarker.Render(dot) + " " + styleMarker.Render(mark) + ts.Render(title)
			if subText != "" {
				line += "  " + styleDim.Render(subText)
			}
			line += spaces(max(gap, 0))
			if tw > 0 {
				line += "  " + styleDim.Render(when)
			}
			line += " "
		}
		out = append(out, marker(sel, focused)+line)
	}
	if m.mon.more && len(out) < h {
		out = append(out, " "+styleDim.Render(m.spinner()+" 正在加载更多…"))
	}
	return fill(out, w, h)
}

func nonEmpty(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// matchLines 是详情页的“命中”区块：逐条列出计划名与命中词。
func matchLines(it row) []summary.Line {
	if !it.mon || len(it.plans) == 0 {
		return nil
	}
	out := []summary.Line{{Kind: summary.Heading, Text: "命中"}}
	for _, p := range it.plans {
		var kws []string
		for _, k := range p.MatchedKeywords {
			if k = ui.CleanText(k); k != "" {
				kws = append(kws, k)
			}
		}
		out = append(out, summary.Line{Kind: summary.Bullet, Label: ui.CleanText(p.PlanName) + "：", Text: strings.Join(kws, "、")})
	}
	return out
}
