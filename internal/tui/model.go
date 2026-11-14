package tui

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/cli"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/ui"
	"github.com/rebang-today/rebang-cli/internal/update"
)

const (
	listLimit   = 100
	searchLimit = 50
	doubleClick = 400 * time.Millisecond
	// wideMinWidth 是双栏的最小宽度：左栏最宽 18 列 + 分隔线 + 右栏至少 37 列
	// （序号、约 17 列标题与热度列）。更窄时退化为单栏。
	wideMinWidth = 56
	requestLimit = 30 * time.Second
)

// Options 是交互界面的运行参数。
type Options struct {
	Source   Source
	Version  string
	LoggedIn bool
	// Refresh 为 0 时不定时刷新。
	Refresh    time.Duration
	AutoUpdate bool
	Open       func(string) error
	Copy       func(string) error
	Now        func() time.Time
	// Pointer 用于写出鼠标指针形状的控制序列（OSC 22），为 nil 时不改变指针。
	Pointer io.Writer
	// StatePath 是位置记忆文件的路径，为空时不读写。
	StatePath string
	// Web 是网站地址，用于提示去网站创建监控计划。
	Web string
}

type focus int

const (
	focusTree focus = iota
	focusList
)

// row 是右栏的一行。
type row struct {
	title, url, mobile, meta, desc string
	// key 是条目的 item_key，用于查询 AI 总结；src、when、heat 用于预览与详情的来源、时间、热度。
	key, src, when, heat string
	// 内容监控的命中内容（D-27）：mon 为 true；unread 为未读；match 是命中词；plans 是命中的计划。
	mon    bool
	unread bool
	match  string
	plans  []api.MonitorPlanMatch
}

type model struct {
	opt Options

	width, height int
	focus         focus

	// 菜单与左栏。
	menus       []api.Menu
	menuIdx     int
	data        map[int64]*menuData
	menuLoading map[int64]bool
	menuErrs    map[int64]string
	srcCur      int
	srcOff      int
	subIdx      int
	// 每个菜单上次的来源 key 与每个来源上次手动选的子榜单 key，退出后保存到状态文件。
	srcMem map[int64]string
	subMem map[string]string
	// mem 是启动时读到的记忆，saveSeq 用于合并延迟写入，posSig 是上次安排写入时的位置。
	mem     posState
	saveSeq int
	posSig  string
	// 左栏移动后延迟加载右栏，debSeq 用于丢弃过期的延迟。
	debSeq     int
	debPending bool
	// autoSub 表示当前子榜单是自动选的（没有记忆），而不是用户手动选的。
	autoSub bool
	// pickFrom 是单栏打开来源列表前的来源下标，Esc 时恢复。
	pickFrom int
	// menusSeen 表示已经按记忆选过启动时的菜单。
	menusSeen bool

	target     listTarget
	searchQ    string
	items      []row
	listCur    int
	listOff    int
	listSeq    int
	loading    bool
	access     *api.ContentAccess
	updatedAt  time.Time
	listNotice string

	account  *cli.AccountView
	loggedIn bool
	expired  bool
	newVer   string

	message string
	isError bool

	searching bool
	input     textinput.Model
	help      bool

	lastClickAt  time.Time
	lastClickIdx int
	lastClickPan focus

	// AI 总结（D-26）：shorts 是批量查到的一句话总结，shortSeen 记录已查询过的 item_key（含没有总结的）；
	// insights 是详情页按 item_key 缓存的完整总结；previewOff 表示关闭了双栏底部的预览；det 为打开的详情页。
	shorts     map[string]string
	shortSeen  map[string]bool
	insights   map[string]*insightEntry
	previewOff bool
	det        *detailState

	// mon 是内容监控的数据与分页状态（D-27）。
	mon monitorState

	// pointerOn 表示已把鼠标指针切换为小手。
	pointerOn bool
	// 鼠标悬停的行，hoverOK 为 false 表示没有悬停。
	hoverOK   bool
	hoverPane focus
	hoverIdx  int

	// 加载动画。
	spin     int
	spinning bool

	// listErr 是右栏加载失败的原因，用于空状态展示。
	listErr string
	// menuErr 是菜单列表加载失败的原因，按 r 重试。
	menuErr string
}

// 消息类型。
type (
	menusMsg struct {
		menus []api.Menu
		err   error
	}
	menuDataMsg struct {
		id   int64
		data *menuData
		err  error
	}
	listMsg struct {
		seq int
		res *cli.ListResult
		err error
	}
	searchMsg struct {
		seq int
		res *cli.SearchResult
		err error
	}
	accountMsg struct {
		acct *cli.AccountView
		err  error
	}
	updateMsg struct {
		m   *update.Manifest
		err error
	}
	installedMsg struct {
		version string
		err     error
	}
	tickMsg     struct{}
	spinMsg     struct{}
	debounceMsg struct{ seq int }
)

// debounce 是左栏移动后到加载右栏之间的等待时间，快速移动时只加载最后停下的榜单。
const debounce = 150 * time.Millisecond

func newModel(opt Options) *model {
	if opt.Now == nil {
		opt.Now = time.Now
	}
	in := textinput.New()
	in.Prompt = "搜索：" // 中文提示
	in.Placeholder = "输入关键词，回车搜索，Esc 取消"
	in.CharLimit = 100
	mem := loadPosState(opt.StatePath)
	srcMem, subMem := map[int64]string{}, map[string]string{}
	for k, v := range mem.Sources {
		if id, err := strconv.ParseInt(k, 10, 64); err == nil && v != "" {
			srcMem[id] = v
		}
	}
	for k, v := range mem.Subs {
		if v != "" {
			subMem[k] = v
		}
	}
	return &model{
		opt:         opt,
		shorts:      map[string]string{},
		shortSeen:   map[string]bool{},
		insights:    map[string]*insightEntry{},
		previewOff:  mem.Preview != nil && !*mem.Preview,
		mon:         monitorState{unreadOnly: mem.MonitorUnread != nil && *mem.MonitorUnread},
		data:        map[int64]*menuData{},
		menuLoading: map[int64]bool{},
		menuErrs:    map[int64]string{},
		srcMem:      srcMem,
		subMem:      subMem,
		mem:         mem,
		loggedIn:    opt.LoggedIn,
		menuIdx:     1, // 启动时停在第一个服务端菜单（综合）
		input:       in,
		width:       100,
		height:      30,
	}
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), requestLimit)
}

func (m *model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadMenus(), m.tick()}
	if m.loggedIn {
		cmds = append(cmds, m.loadAccount())
	}
	cmds = append(cmds, m.checkUpdate())
	return tea.Batch(cmds...)
}

// ---- 命令 ----

func (m *model) loadMenus() tea.Cmd {
	src := m.opt.Source
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		menus, err := src.Menus(c)
		return menusMsg{menus, err}
	}
}

func (m *model) loadMenu(id int64) tea.Cmd {
	m.menuLoading[id] = true
	delete(m.menuErrs, id)
	src := m.opt.Source
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		d, err := src.MenuData(c, id)
		return menuDataMsg{id, d, err}
	}
}

func (m *model) loadAccount() tea.Cmd {
	src := m.opt.Source
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		a, err := src.Account(c)
		return accountMsg{a, err}
	}
}

func (m *model) checkUpdate() tea.Cmd {
	src := m.opt.Source
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		mf, err := src.CheckUpdate(c)
		return updateMsg{mf, err}
	}
}

func (m *model) install(mf *update.Manifest) tea.Cmd {
	src := m.opt.Source
	return func() tea.Msg {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		return installedMsg{mf.Version, src.Install(c, mf)}
	}
}

func (m *model) tick() tea.Cmd {
	if m.opt.Refresh <= 0 {
		return nil
	}
	return tea.Tick(m.opt.Refresh, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m *model) loadList(t listTarget) tea.Cmd {
	m.listSeq++
	seq := m.listSeq
	if t.key() != m.target.key() || m.searchQ != "" {
		m.items, m.listCur, m.listOff, m.access, m.listNotice = nil, 0, 0, nil, ""
	}
	m.target, m.searchQ, m.loading = t, "", true
	src := m.opt.Source
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		res, err := src.List(c, t, listLimit)
		return listMsg{seq, res, err}
	}
}

func (m *model) runSearch(q string) tea.Cmd {
	m.listSeq++
	seq := m.listSeq
	if q != m.searchQ {
		m.items, m.listCur, m.listOff = nil, 0, 0
	}
	m.searchQ, m.access, m.listNotice, m.loading = q, nil, "", true
	src := m.opt.Source
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		res, err := src.Search(c, q, searchLimit)
		return searchMsg{seq, res, err}
	}
}

func (m *model) refresh() tea.Cmd {
	switch {
	case m.searchQ != "":
		return m.runSearch(m.searchQ)
	case m.inMonitor():
		return m.loadMonitor(false)
	case m.target.key() != "||":
		return m.loadList(m.target)
	}
	return nil
}

// ---- 更新 ----

const spinEvery = 100 * time.Millisecond

// Update 处理消息；加载期间驱动转圈动画。
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(spinMsg); ok {
		m.spin++
		if !m.busy() {
			m.spinning = false
			return m, nil
		}
		return m, m.spinTick()
	}
	if sm, ok := msg.(saveMsg); ok {
		if sm.seq == m.saveSeq {
			m.SaveState()
		}
		return m, nil
	}
	next, cmd := m.update(msg)
	if more := m.maybeMore(); more != nil {
		cmd = tea.Batch(cmd, more)
	}
	if sig := m.posSignature(); sig != m.posSig {
		// 启动后第一次得到位置只是恢复记忆，不必写入。
		if m.posSig != "" {
			cmd = tea.Batch(cmd, m.markDirty())
		}
		m.posSig = sig
	}
	if m.busy() && !m.spinning {
		m.spinning = true
		cmd = tea.Batch(cmd, m.spinTick())
	}
	return next, cmd
}

// busy 表示有正在进行的加载，需要转圈动画。
func (m *model) busy() bool {
	if m.loading || m.detailLoading() || (len(m.menus) == 0 && m.menuErr == "") {
		return true
	}
	if mn, ok := m.curMenu(); ok && m.menuLoading[mn.ID] {
		return true
	}
	return false
}

func (m *model) spinTick() tea.Cmd {
	return tea.Tick(spinEvery, func(time.Time) tea.Msg { return spinMsg{} })
}

func (m *model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		wasWide, wasPicking := m.wide(), m.picking()
		m.width, m.height = msg.Width, msg.Height
		switch {
		case wasWide && !m.wide() && m.focus == focusTree:
			// 变成单栏时默认显示榜单，来源列表按 tab 打开。
			m.focus = focusList
		case wasPicking && m.wide():
			// 正在单栏里选来源时变宽：放弃未确认的选择。
			m.srcCur = m.pickFrom
		}
		m.clamp()
		m.clampDetail()
		return m, nil
	case tea.KeyMsg:
		return m, m.onKey(msg)
	case tea.MouseMsg:
		return m, m.onMouse(msg)
	case menusMsg:
		if msg.err != nil {
			m.menuErr = errText(msg.err)
			m.fail(msg.err)
			return m, nil
		}
		m.menuErr = ""
		// “订阅”固定在最前面（数字键 0），其余是服务端的菜单。
		m.menus = append([]api.Menu{{ID: followMenuID, Name: followMenuName}}, msg.menus...)
		for i := range m.menus {
			m.menus[i].Name = ui.CleanText(m.menus[i].Name)
		}
		if !m.menusSeen {
			// 第一次拿到菜单：回到上次的菜单；它已不存在时停在第一个服务端菜单。
			m.menusSeen = true
			if m.mem.Menu != nil {
				for i, mn := range m.menus {
					if mn.ID == *m.mem.Menu {
						m.menuIdx = i
					}
				}
			}
		}
		m.menuIdx = clampIdx(m.menuIdx, len(m.menus))
		if len(m.menus) == 1 {
			m.menuIdx = 0
		}
		return m, m.enterMenu(m.menuIdx)
	case menuDataMsg:
		delete(m.menuLoading, msg.id)
		if msg.err != nil {
			m.menuErrs[msg.id] = errText(msg.err)
			m.fail(msg.err)
			return m, nil
		}
		if msg.data == nil {
			msg.data = &menuData{}
		}
		m.data[msg.id] = cleanMenuData(msg.data)
		var unread tea.Cmd
		if msg.id == followMenuID && m.hasMonitorSource(m.data[msg.id]) {
			// 左栏“内容监控”后显示总未读数。
			unread = m.loadMonitorUnread()
		}
		if mn, ok := m.curMenu(); ok && mn.ID == msg.id {
			m.restorePos()
			return m, tea.Batch(m.loadCurrent(), unread)
		}
		return m, unread
	case debounceMsg:
		if msg.seq != m.debSeq || !m.debPending {
			return m, nil
		}
		return m, m.loadCurrent()
	case accountMsg:
		if msg.err != nil {
			if isAuth(msg.err) {
				m.markExpired()
			}
			return m, nil
		}
		m.account = msg.acct
		return m, nil
	case listMsg:
		if msg.seq != m.listSeq {
			return m, nil
		}
		m.loading = false
		if msg.err != nil && (msg.res == nil || len(msg.res.Items) == 0) {
			m.items = nil
			m.listErr = errText(msg.err)
			m.fail(msg.err)
			return m, nil
		}
		m.listErr = ""
		m.setList(msg.res)
		return m, tea.Batch(m.wechatFallback(), m.loadShorts())
	case searchMsg:
		if msg.seq != m.listSeq {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.items = nil
			m.listErr = errText(msg.err)
			m.fail(msg.err)
			return m, nil
		}
		m.listErr = ""
		m.setSearch(msg.res)
		return m, m.loadShorts()
	case updateMsg:
		if msg.err != nil || msg.m == nil {
			return m, nil
		}
		m.newVer = msg.m.Version
		if msg.m.MinVersion != "" && update.Compare(m.opt.Version, msg.m.MinVersion) < 0 {
			m.flash(errs.New(errs.ExitUpgrade, errs.CodeUpgradeRequired, "当前版本已不再受支持", "退出后运行 rebang update 升级").Error(), true)
		}
		if m.opt.AutoUpdate {
			m.flash("正在后台升级到 "+msg.m.Version+" …", false)
			return m, m.install(msg.m)
		}
		return m, nil
	case installedMsg:
		if msg.err != nil {
			m.flash("自动升级失败："+errText(msg.err), true)
		} else {
			m.newVer = ""
			m.flash("已升级到 "+msg.version+"，重启 rebang 生效", false)
		}
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.refresh(), m.tick())
	case shortsMsg:
		m.onShorts(msg)
		return m, nil
	case insightMsg:
		m.onInsight(msg)
		return m, nil
	case monitorMsg:
		return m, m.onMonitor(msg)
	case monitorMoreMsg:
		return m, m.onMonitorMore(msg)
	case monitorUnreadMsg:
		m.onMonitorUnread(msg)
		return m, nil
	}
	if m.searching {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *model) setList(res *cli.ListResult) {
	keepURL := m.selectedURL()
	m.items = m.items[:0]
	for _, it := range res.Items {
		meta := it.Heat
		if meta == "" && it.Label != "" {
			meta = it.Label
		}
		m.items = append(m.items, row{title: ui.CleanText(it.Title), url: it.URL, mobile: it.MobileURL, meta: ui.CleanText(meta), desc: ui.CleanText(it.Desc),
			key: it.ItemKey, src: m.listSourceName(), when: shortTime(it.PublishedAt, m.opt.Now()), heat: ui.CleanText(it.Heat)})
	}
	m.access = res.ContentAccess
	m.listNotice = ""
	if a := res.ContentAccess; a != nil && a.Mode != "" && a.Mode != "full" {
		m.listNotice = "非会员仅展示部分内容，开通会员查看完整榜单"
		if a.Reason != "" {
			m.listNotice = a.Reason
		}
	}
	m.updatedAt = m.opt.Now()
	m.restoreCursor(keepURL)
}

func (m *model) setSearch(res *cli.SearchResult) {
	keepURL := m.selectedURL()
	m.items = m.items[:0]
	for _, it := range res.Items {
		src := it.SourceName
		if src == "" {
			src = it.Source
		}
		m.items = append(m.items, row{title: ui.CleanText(it.Title), url: it.URL, mobile: it.MobileURL, meta: ui.CleanText(src), desc: ui.CleanText(it.Snippet),
			key: it.ItemKey, src: ui.CleanText(src), when: shortTime(it.LastSeen, m.opt.Now())})
	}
	if len(m.items) == 0 {
		m.listNotice = "没有找到相关内容"
	}
	m.updatedAt = m.opt.Now()
	m.restoreCursor(keepURL)
}

// restoreCursor 刷新后尽量停留在原来那条内容上。
func (m *model) restoreCursor(url string) {
	if url != "" {
		for i, it := range m.items {
			if it.url == url {
				m.listCur = i
				m.clamp()
				return
			}
		}
	}
	if m.listCur >= len(m.items) {
		m.listCur = 0
	}
	m.clamp()
}

func (m *model) selectedURL() string {
	if m.listCur >= 0 && m.listCur < len(m.items) {
		return m.items[m.listCur].url
	}
	return ""
}

func (m *model) markExpired() {
	m.expired = true
	m.account = nil
	m.flash("登录已失效，退出后运行 rebang auth login 重新登录", true)
}

func (m *model) fail(err error) {
	if isAuth(err) {
		m.markExpired()
		return
	}
	m.flash(errText(err), true)
}

func (m *model) flash(s string, isErr bool) { m.message, m.isError = ui.CleanText(s), isErr }

// cleanMenuData 清洗菜单数据中要显示的名称，避免不可见字符打乱终端排版。
func cleanMenuData(d *menuData) *menuData {
	for i := range d.tabs {
		t := &d.tabs[i]
		t.Name = ui.CleanText(t.Name)
		for j := range t.Children {
			t.Children[j].Name = ui.CleanText(t.Children[j].Name)
		}
	}
	for i := range d.nodes {
		d.nodes[i].Name = ui.CleanText(d.nodes[i].Name)
	}
	return d
}

func errText(err error) string {
	var e *errs.Error
	if errors.As(err, &e) {
		if e.Hint != "" {
			return e.Message + "（" + e.Hint + "）"
		}
		return e.Message
	}
	return err.Error()
}

func isAuth(err error) bool { return errs.ExitCode(err) == errs.ExitAuth }

// ---- 导航 ----

func (m *model) curMenu() (api.Menu, bool) {
	if m.menuIdx < 0 || m.menuIdx >= len(m.menus) {
		return api.Menu{}, false
	}
	return m.menus[m.menuIdx], true
}

// curSources 返回当前菜单的左栏来源；菜单数据未加载时为 nil。
func (m *model) curSources() []source {
	mn, ok := m.curMenu()
	if !ok || m.data[mn.ID] == nil {
		return nil
	}
	return sources(m.data[mn.ID])
}

func (m *model) curSource() source {
	srcs := m.curSources()
	if len(srcs) == 0 {
		return source{}
	}
	return srcs[clampIdx(m.srcCur, len(srcs))]
}

// curSubs 返回当前来源的子榜单；搜索时不显示。
func (m *model) curSubs() []subTab {
	if m.searchQ != "" {
		return nil
	}
	return m.sourceSubs()
}

// sourceSubs 返回当前来源的子榜单；内容监控的子榜单是“全部”与各个计划。
func (m *model) sourceSubs() []subTab {
	if s := m.curSource(); s.monitoring() {
		return m.monitorSubs()
	}
	return m.curSource().subs()
}

func (m *model) memKey(s source) string {
	mn, _ := m.curMenu()
	return itoa(mn.ID) + "/" + s.key()
}

// savePos 记住当前菜单的来源与当前来源（手动选过）的子榜单。
// 单栏来源列表打开时，记住的是打开前的来源，未确认的选择不保存。
func (m *model) savePos() {
	if m.picking() {
		cur := m.srcCur
		m.srcCur = m.pickFrom
		defer func() { m.srcCur = cur }()
	}
	mn, ok := m.curMenu()
	if !ok || m.data[mn.ID] == nil {
		return
	}
	s := m.curSource()
	if s.kind == srcNone {
		return
	}
	m.srcMem[mn.ID] = s.key()
	// 自动选的子榜单不记忆，下次进入时重新按默认规则选择。
	if !m.autoSub && s.kind == srcTab && len(s.tab.Children) > 0 {
		m.subMem[m.memKey(s)] = s.tab.Children[clampIdx(m.subIdx, len(s.tab.Children))].Key
	}
}

// restorePos 恢复菜单上次的来源；没有记忆或该来源已不存在时选第一个榜单。
func (m *model) restorePos() {
	mn, _ := m.curMenu()
	srcs := m.curSources()
	m.srcCur = 0
	if k, ok := m.srcMem[mn.ID]; ok {
		for i, s := range srcs {
			if s.key() == k {
				m.srcCur = i
				break
			}
		}
	}
	m.srcCur = clampIdx(m.srcCur, len(srcs))
	m.subIdx, m.autoSub = m.recallSub(m.curSource())
	m.clamp()
}

// posSignature 概括需要持久化的位置，变化时才安排写入。
func (m *model) posSignature() string {
	if len(m.menus) == 0 {
		return ""
	}
	mn, _ := m.curMenu()
	sig := itoa(mn.ID) + "|" + itoa(int64(len(m.srcMem))) + "|" + itoa(int64(len(m.subMem)))
	if m.previewOff {
		sig += "|nopreview"
	}
	if m.mon.unreadOnly {
		sig += "|unread"
	}
	if k, ok := m.srcMem[mn.ID]; ok {
		sig += "|" + k
		if sub, ok := m.subMem[itoa(mn.ID)+"/"+k]; ok {
			sig += "|" + sub
		}
	}
	return sig
}

// 公众号来源的子榜单规则，与网站一致：默认“关注”，未登录或关注为空时改用“热门”。
const (
	wechatTab       = "wechat"
	wechatFollowing = "following"
	wechatHot       = "hot"
)

func subIndex(s source, key string) int {
	for i, c := range s.tab.Children {
		if c.Key == key {
			return i
		}
	}
	return -1
}

// recallSub 返回来源上次的子榜单；没有记忆时按默认规则选择，auto 为 true。
func (m *model) recallSub(s source) (idx int, auto bool) {
	if k, ok := m.subMem[m.memKey(s)]; ok {
		// 记住的子榜单已不存在时当作没有记忆。
		if i := subIndex(s, k); i >= 0 {
			return i, false
		}
	}
	if s.kind == srcTab && s.tab.Key == wechatTab {
		hot := subIndex(s, wechatHot)
		if hot >= 0 && (!m.loggedIn || m.expired) {
			return hot, true
		}
		if f := subIndex(s, wechatFollowing); f >= 0 {
			return f, true
		}
	}
	return 0, true
}

// wechatFallback 在自动进入公众号“关注”且结果为空时切到“热门”，并记为该来源的子榜单。
func (m *model) wechatFallback() tea.Cmd {
	s := m.curSource()
	if !m.autoSub || m.searchQ != "" || len(m.items) != 0 || s.kind != srcTab || s.tab.Key != wechatTab || m.target.subTab != wechatFollowing {
		return nil
	}
	hot := subIndex(s, wechatHot)
	if hot < 0 {
		return nil
	}
	m.subIdx, m.autoSub = hot, false
	m.savePos()
	return m.loadCurrent()
}

// enterMenu 切换到第 i 个菜单并加载它上次停留的榜单，不改变焦点。
func (m *model) enterMenu(i int) tea.Cmd {
	if len(m.menus) == 0 {
		return nil
	}
	i = clampIdx(i, len(m.menus))
	m.savePos()
	m.menuIdx, m.srcOff, m.hoverOK = i, 0, false
	mn := m.menus[i]
	if mn.ID == followMenuID && (!m.loggedIn || m.expired) {
		// 未登录时订阅为空，不发请求。
		m.data[mn.ID] = &menuData{}
	}
	if m.data[mn.ID] == nil {
		m.clearList()
		if m.menuLoading[mn.ID] {
			return nil
		}
		return m.loadMenu(mn.ID)
	}
	m.restorePos()
	return m.loadCurrent()
}

// clearList 清空右栏并丢弃尚未返回的请求。
func (m *model) clearList() {
	m.debSeq++
	m.debPending = false
	m.listSeq++
	m.target, m.searchQ = listTarget{}, ""
	m.items, m.listCur, m.listOff, m.access, m.listNotice, m.listErr = nil, 0, 0, nil, "", ""
	m.loading = false
}

// loadCurrent 立即加载当前来源与子榜单对应的榜单。
func (m *model) loadCurrent() tea.Cmd {
	m.debSeq++
	m.debPending = false
	src := m.curSource()
	if src.monitoring() {
		m.searchQ = ""
		return m.loadMonitor(false)
	}
	t, ok := src.target(m.subIdx)
	if !ok {
		m.clearList()
		return nil
	}
	m.listErr = ""
	return m.loadList(t)
}

// moveSource 移动左栏光标，停下 debounce 后再加载右栏；焦点留在左栏。
func (m *model) moveSource(d int) tea.Cmd {
	srcs := m.curSources()
	if len(srcs) == 0 {
		return nil
	}
	next := clampIdx(m.srcCur+d, len(srcs))
	if next == m.srcCur {
		m.clamp()
		return nil
	}
	m.srcCur = next
	m.subIdx, m.autoSub = m.recallSub(m.curSource())
	m.savePos()
	m.clamp()
	t, ok := m.curSource().target(m.subIdx)
	if !ok {
		return m.loadCurrent()
	}
	// 先清空右栏并丢弃旧请求，避免显示上一个榜单的内容。
	m.listSeq++
	m.items, m.listCur, m.listOff, m.access, m.listNotice, m.listErr = nil, 0, 0, nil, "", ""
	m.target, m.searchQ, m.loading = t, "", true
	m.debSeq++
	m.debPending = true
	seq := m.debSeq
	return tea.Tick(debounce, func(time.Time) tea.Msg { return debounceMsg{seq} })
}

// selectSource 直接选中左栏第 i 项并立即加载（鼠标点击）。
func (m *model) selectSource(i int) tea.Cmd {
	srcs := m.curSources()
	if i < 0 || i >= len(srcs) {
		return nil
	}
	if i == m.srcCur && m.searchQ == "" && !m.debPending && m.target.key() != "||" {
		return nil
	}
	m.srcCur = i
	m.subIdx, m.autoSub = m.recallSub(m.curSource())
	m.savePos()
	m.clamp()
	return m.loadCurrent()
}

// setSub 切换到第 i 个子榜单并立即加载。
func (m *model) setSub(i int) tea.Cmd {
	subs := m.sourceSubs()
	if len(subs) == 0 {
		return nil
	}
	i = clampIdx(i, len(subs))
	if i == m.subIdx && m.searchQ == "" && !m.debPending {
		return nil
	}
	m.subIdx, m.autoSub = i, false
	m.savePos()
	return m.loadCurrent()
}

// ---- 键盘 ----

func (m *model) onKey(k tea.KeyMsg) tea.Cmd {
	key := k.String()
	if key == "ctrl+c" {
		return tea.Quit
	}
	if m.det != nil {
		return m.detailKey(key)
	}
	if m.searching {
		switch key {
		case "esc":
			m.searching = false
			m.input.Blur()
			return nil
		case "enter":
			q := strings.TrimSpace(m.input.Value())
			m.searching = false
			m.input.Blur()
			if q == "" {
				return nil
			}
			m.focus = focusList
			m.debSeq++
			m.debPending = false
			return m.runSearch(q)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(k)
		return cmd
	}
	if m.help {
		if key == "q" {
			return tea.Quit
		}
		m.help = false
		return nil
	}
	m.message = ""
	if m.picking() {
		if cmd, done := m.pickerKey(key); done {
			return cmd
		}
	}
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		// 0 是订阅，1～9 是服务端菜单（m.menus 的下标与数字一致）。
		if i := int(key[0] - '0'); i < len(m.menus) && i != m.menuIdx {
			return m.enterMenu(i)
		}
		return nil
	}
	switch key {
	case "q":
		return tea.Quit
	case "?":
		m.help = true
	case "/":
		m.searching = true
		m.input.SetValue("")
		return m.input.Focus()
	case "esc":
		if m.searchQ != "" {
			return m.loadCurrent()
		}
		if m.wide() {
			m.focus = focusTree
		}
	case "tab", "shift+tab":
		if !m.wide() {
			m.openPicker()
			return nil
		}
		if m.focus == focusTree {
			m.focus = focusList
		} else {
			m.focus = focusTree
		}
	case "[":
		if len(m.menus) > 0 {
			return m.enterMenu((m.menuIdx - 1 + len(m.menus)) % len(m.menus))
		}
	case "]":
		if len(m.menus) > 0 {
			return m.enterMenu((m.menuIdx + 1) % len(m.menus))
		}
	case "r":
		m.flash("正在刷新…", false)
		var cmds []tea.Cmd
		mn, ok := m.curMenu()
		switch {
		case len(m.menus) == 0:
			m.menuErr = ""
			cmds = append(cmds, m.loadMenus())
		case ok && (m.data[mn.ID] == nil || (mn.ID == followMenuID && m.loggedIn && !m.expired)):
			// 菜单未加载或加载失败时重新加载；订阅菜单同时刷新订阅列表。
			if !m.menuLoading[mn.ID] {
				cmds = append(cmds, m.loadMenu(mn.ID))
			}
		default:
			cmds = append(cmds, m.refresh())
		}
		if m.loggedIn && !m.expired {
			cmds = append(cmds, m.loadAccount())
		}
		return tea.Batch(cmds...)
	case "y":
		return m.copySelected()
	case "left", "h":
		return m.setSub(m.subIdx - 1)
	case "right", "l":
		return m.setSub(m.subIdx + 1)
	case "up", "k":
		return m.move(-1)
	case "down", "j":
		return m.move(1)
	case "K", "shift+up":
		// 在榜单里直接换来源：左栏光标同步移动，焦点不变，停下 debounce 后再加载。
		return m.moveSource(-1)
	case "J", "shift+down":
		return m.moveSource(1)
	case "pgup", "ctrl+b":
		return m.move(-m.paneHeight())
	case "pgdown", "ctrl+f":
		return m.move(m.paneHeight())
	case " ", "space":
		return m.openDetail()
	case "p":
		m.togglePreview()
	case "u":
		return m.toggleUnread()
	case "home", "g":
		return m.move(-1 << 20)
	case "end", "G":
		return m.move(1 << 20)
	case "enter":
		if m.focus == focusList {
			return m.openSelected()
		}
		m.focus = focusList
		if m.debPending {
			return m.loadCurrent()
		}
	}
	return nil
}

// picking 表示单栏模式下打开了来源列表（占满整栏）。
func (m *model) picking() bool { return !m.wide() && m.focus == focusTree }

// openPicker 在单栏模式下打开来源列表。
func (m *model) openPicker() {
	m.pickFrom, m.focus, m.hoverOK = m.srcCur, focusTree, false
	m.clamp()
}

// cancelPick 关闭来源列表并回到原来的来源，不切换。
func (m *model) cancelPick() {
	m.srcCur, m.focus = m.pickFrom, focusList
	m.clamp()
}

// commitPick 关闭来源列表并切换到第 i 个来源。
func (m *model) commitPick(i int) tea.Cmd {
	m.srcCur, m.focus = m.pickFrom, focusList
	if i == m.pickFrom && m.searchQ == "" && m.target.key() != "||" {
		m.clamp()
		return nil
	}
	return m.selectSource(i)
}

// pickerKey 处理来源列表打开时的按键；done 为 false 时交给普通按键处理。
func (m *model) pickerKey(key string) (cmd tea.Cmd, done bool) {
	n := len(m.curSources())
	step := func(d int) {
		if n > 0 {
			m.srcCur = clampIdx(m.srcCur+d, n)
		}
		m.clamp()
	}
	switch key {
	case "up", "k", "K", "shift+up":
		step(-1)
	case "down", "j", "J", "shift+down":
		step(1)
	case "pgup", "ctrl+b":
		step(-m.bodyHeight())
	case "pgdown", "ctrl+f", " ":
		step(m.bodyHeight())
	case "home", "g":
		step(-1 << 20)
	case "end", "G":
		step(1 << 20)
	case "enter":
		return m.commitPick(m.srcCur), true
	case "esc", "tab", "shift+tab":
		m.cancelPick()
	case "left", "h", "right", "l", "y":
	case "q", "?", "r":
		return nil, false
	default:
		// 切换分类、搜索等：先关闭来源列表，再按普通按键处理。
		m.cancelPick()
		return nil, false
	}
	return nil, true
}

// move 在当前焦点栏内移动；左栏移动会延迟加载右栏。
func (m *model) move(d int) tea.Cmd {
	if m.focus == focusTree {
		return m.moveSource(d)
	}
	m.listCur += d
	m.clamp()
	return nil
}

// paneHeight 是当前焦点栏可见的行数，用于翻页。
func (m *model) paneHeight() int {
	if m.focus == focusTree {
		return m.bodyHeight()
	}
	return m.listHeight()
}

func (m *model) selected() (row, bool) {
	if m.listCur < 0 || m.listCur >= len(m.items) {
		return row{}, false
	}
	return m.items[m.listCur], true
}

func (m *model) openSelected() tea.Cmd {
	it, _ := m.selected()
	return m.openItem(it)
}

// openItem 用浏览器打开条目；内容监控的未读命中同时标为已读（不等待、不阻塞打开）。
func (m *model) openItem(it row) tea.Cmd {
	open := m.openURL(it.url)
	if it.url == "" {
		return open
	}
	return tea.Batch(open, m.markRead(it))
}

func (m *model) openURL(u string) tea.Cmd {
	if u == "" || m.opt.Open == nil {
		return nil
	}
	if err := m.opt.Open(u); err != nil {
		m.flash("无法打开浏览器："+u, true)
		return nil
	}
	m.flash("已在浏览器中打开", false)
	return nil
}

func (m *model) copySelected() tea.Cmd {
	it, _ := m.selected()
	return m.copyURL(it.url)
}

func (m *model) copyURL(u string) tea.Cmd {
	if u == "" {
		return nil
	}
	if m.opt.Copy == nil || m.opt.Copy(u) != nil {
		m.flash("无法访问剪贴板，链接："+u, true)
		return nil
	}
	m.flash("已复制链接", false)
	return nil
}

// ---- 鼠标 ----

// hit 描述屏幕上一个位置对应的元素。
type hit struct {
	what int // hitNone、hitMenu、hitSub、hitSource、hitItem
	idx  int
	pane focus
}

const (
	hitNone = iota
	hitMenu
	hitSub
	hitSource
	hitItem
)

// hitTest 把鼠标坐标换算为界面元素，与 View 的布局保持一致。
func (m *model) hitTest(x, y int) hit {
	if y == 0 {
		for _, sp := range m.menuSpans() {
			if x >= sp.x0 && x < sp.x1 {
				return hit{what: hitMenu, idx: sp.idx}
			}
		}
		return hit{}
	}
	pane, ok := m.paneAt(x)
	if !ok || y > m.bodyHeight() {
		return hit{}
	}
	if pane == focusTree {
		idx := m.srcOff + y - 1
		if idx < len(m.curSources()) {
			return hit{what: hitSource, idx: idx, pane: pane}
		}
		return hit{pane: pane}
	}
	x0 := m.rightX()
	if m.hasStrip() && y == 2 {
		for _, sp := range m.stripSpans(x0) {
			if x >= sp.x0 && x < sp.x1 {
				return hit{what: hitSub, idx: sp.idx, pane: pane}
			}
		}
		return hit{pane: pane}
	}
	idx := m.listOff + y - 1 - m.rightHead()
	if y-1 >= m.rightHead() && y-1-m.rightHead() < m.listHeight() && idx < len(m.items) {
		return hit{what: hitItem, idx: idx, pane: pane}
	}
	return hit{pane: pane}
}

func (m *model) onMouse(ev tea.MouseMsg) tea.Cmd {
	if m.tooSmall() || m.help || m.searching {
		m.setPointer(false)
		return nil
	}
	if m.det != nil {
		m.setPointer(false)
		if ev.Action == tea.MouseActionPress && (ev.Button == tea.MouseButtonWheelUp || ev.Button == tea.MouseButtonWheelDown) {
			d := 3
			if ev.Button == tea.MouseButtonWheelUp {
				d = -3
			}
			m.det.off += d
			m.clampDetail()
		}
		return nil
	}
	h := m.hitTest(ev.X, ev.Y)
	m.setPointer(h.what != hitNone)
	if ev.Action == tea.MouseActionMotion && ev.Button == tea.MouseButtonNone {
		m.hoverOK = false
		if h.what == hitSource || h.what == hitItem {
			m.hoverOK, m.hoverPane, m.hoverIdx = true, h.pane, h.idx
		}
		return nil
	}
	switch ev.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		if ev.Action != tea.MouseActionPress || ev.Y == 0 {
			return nil
		}
		pane, ok := m.paneAt(ev.X)
		if !ok {
			return nil
		}
		d := 3
		if ev.Button == tea.MouseButtonWheelUp {
			d = -3
		}
		m.scroll(pane, d)
		return nil
	case tea.MouseButtonLeft:
		if ev.Action != tea.MouseActionPress {
			return nil
		}
	default:
		return nil
	}
	now := m.opt.Now()
	switch h.what {
	case hitMenu:
		if h.idx != m.menuIdx {
			return m.enterMenu(h.idx)
		}
	case hitSub:
		return m.setSub(h.idx)
	case hitSource:
		if !m.wide() {
			return m.commitPick(h.idx)
		}
		m.focus = focusTree
		m.lastClickAt, m.lastClickIdx, m.lastClickPan = now, h.idx, focusTree
		return m.selectSource(h.idx)
	case hitItem:
		double := m.lastClickPan == focusList && m.lastClickIdx == h.idx && now.Sub(m.lastClickAt) <= doubleClick
		m.focus, m.listCur = focusList, h.idx
		m.lastClickAt, m.lastClickIdx, m.lastClickPan = now, h.idx, focusList
		if double {
			m.lastClickAt = time.Time{}
			return m.openSelected()
		}
	}
	return nil
}

func (m *model) hovered(pane focus, idx int) bool {
	return m.hoverOK && m.hoverPane == pane && m.hoverIdx == idx
}

func (m *model) scroll(pane focus, d int) {
	m.hoverOK = false
	if pane == focusTree {
		h, n := m.bodyHeight(), len(m.curSources())
		m.srcOff = clampOff(m.srcOff+d, n, h)
		return
	}
	h := m.listHeight()
	m.listOff = clampOff(m.listOff+d, len(m.items), h)
	m.listCur = clampInto(m.listCur, m.listOff, h, len(m.items))
}

func clampOff(off, n, h int) int {
	if off > n-h {
		off = n - h
	}
	if off < 0 {
		off = 0
	}
	return off
}

func clampInto(cur, off, h, n int) int {
	if cur < off {
		cur = off
	}
	if cur > off+h-1 {
		cur = off + h - 1
	}
	if cur >= n {
		cur = n - 1
	}
	if cur < 0 {
		cur = 0
	}
	return cur
}

// clamp 保证光标合法且可见。
func (m *model) clamp() {
	m.srcCur, m.srcOff = fit(m.srcCur, m.srcOff, len(m.curSources()), m.bodyHeight())
	m.listCur, m.listOff = fit(m.listCur, m.listOff, len(m.items), m.listHeight())
}

func fit(cur, off, n, h int) (int, int) {
	if cur >= n {
		cur = n - 1
	}
	if cur < 0 {
		cur = 0
	}
	if cur < off {
		off = cur
	}
	if h > 0 && cur >= off+h {
		off = cur - h + 1
	}
	return cur, clampOff(off, n, h)
}
