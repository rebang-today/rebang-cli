package tui

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/cli"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/update"
)

const (
	listLimit    = 100
	searchLimit  = 50
	doubleClick  = 400 * time.Millisecond
	wideMinWidth = 70
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
}

type focus int

const (
	focusTree focus = iota
	focusList
)

// row 是右栏的一行。
type row struct {
	title, url, mobile, meta, desc string
}

type model struct {
	opt Options

	width, height int
	focus         focus

	tree     *tree
	rows     []entry
	treeCur  int
	treeOff  int
	autoPick bool

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

	// 鼠标悬停的行，hoverOK 为 false 表示没有悬停。
	hoverOK   bool
	hoverPane focus
	hoverIdx  int

	// 加载动画。
	spin     int
	spinning bool

	// listErr 是右栏加载失败的原因，用于空状态展示。
	listErr string
	// menuErr 是菜单加载失败的原因，按 r 重试。
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
	followingMsg struct {
		nodes []api.Node
		err   error
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
	tickMsg struct{}
	spinMsg struct{}
)

func newModel(opt Options) *model {
	if opt.Now == nil {
		opt.Now = time.Now
	}
	in := textinput.New()
	in.Prompt = "搜索：" // 中文提示
	in.Placeholder = "输入关键词，回车搜索，Esc 取消"
	in.CharLimit = 100
	return &model{
		opt:      opt,
		tree:     newTree(),
		loggedIn: opt.LoggedIn,
		autoPick: true,
		input:    in,
		width:    100,
		height:   30,
	}
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), requestLimit)
}

func (m *model) Init() tea.Cmd {
	m.tree.loggedIn = m.loggedIn
	cmds := []tea.Cmd{m.loadMenus(), m.tick()}
	if m.loggedIn {
		cmds = append(cmds, m.loadAccount(), m.loadFollowing())
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
	src := m.opt.Source
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		d, err := src.MenuData(c, id)
		return menuDataMsg{id, d, err}
	}
}

func (m *model) loadFollowing() tea.Cmd {
	src := m.opt.Source
	return func() tea.Msg {
		c, cancel := ctx()
		defer cancel()
		n, err := src.Following(c)
		return followingMsg{n, err}
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
	next, cmd := m.update(msg)
	if m.busy() && !m.spinning {
		m.spinning = true
		cmd = tea.Batch(cmd, m.spinTick())
	}
	return next, cmd
}

// busy 表示有正在进行的加载，需要转圈动画。
func (m *model) busy() bool { return m.loading || (len(m.tree.menus) == 0 && m.menuErr == "") }

func (m *model) spinTick() tea.Cmd {
	return tea.Tick(spinEvery, func(time.Time) tea.Msg { return spinMsg{} })
}

func (m *model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clamp()
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
		m.tree.menus = msg.menus
		m.rebuild()
		if len(msg.menus) > 0 {
			first := msg.menus[0].ID
			m.tree.expanded[menuKey(first)] = true
			m.rebuild()
			return m, m.loadMenu(first)
		}
		return m, nil
	case menuDataMsg:
		if msg.err != nil {
			m.fail(msg.err)
			return m, nil
		}
		m.tree.data[msg.id] = msg.data
		m.rebuild()
		if m.autoPick && m.target.key() == "||" && m.searchQ == "" {
			for i, e := range m.rows {
				if t, ok := e.target(); ok && e.menuID == msg.id {
					m.autoPick = false
					m.treeCur = i
					m.focus = focusList
					if e.kind == kindTab && e.expandable {
						return m, m.activate(e)
					}
					return m, m.loadList(t)
				}
			}
		}
		return m, nil
	case followingMsg:
		if msg.err == nil {
			m.tree.following = msg.nodes
			m.rebuild()
		} else if isAuth(msg.err) {
			m.markExpired()
		}
		return m, nil
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
		return m, nil
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
		return m, nil
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
		m.items = append(m.items, row{title: it.Title, url: it.URL, mobile: it.MobileURL, meta: meta, desc: it.Desc})
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
		m.items = append(m.items, row{title: it.Title, url: it.URL, mobile: it.MobileURL, meta: src, desc: it.Snippet})
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

func (m *model) rebuild() {
	m.rows = m.tree.rows()
	m.clamp()
}

func (m *model) fail(err error) {
	if isAuth(err) {
		m.markExpired()
		return
	}
	m.flash(errText(err), true)
}

func (m *model) flash(s string, isErr bool) { m.message, m.isError = s, isErr }

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

// ---- 键盘 ----

func (m *model) onKey(k tea.KeyMsg) tea.Cmd {
	key := k.String()
	if key == "ctrl+c" {
		return tea.Quit
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
		if m.searchQ != "" && m.target.key() != "||" {
			return m.loadList(m.target)
		}
	case "tab":
		if m.focus == focusTree {
			m.focus = focusList
		} else {
			m.focus = focusTree
		}
	case "r":
		m.flash("正在刷新…", false)
		cmds := []tea.Cmd{m.refresh()}
		if len(m.tree.menus) == 0 {
			m.menuErr = ""
			cmds = append(cmds, m.loadMenus())
		}
		if m.loggedIn && !m.expired {
			cmds = append(cmds, m.loadAccount(), m.loadFollowing())
		}
		return tea.Batch(cmds...)
	case "y":
		return m.copySelected()
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup", "ctrl+b":
		m.move(-m.bodyHeight())
	case "pgdown", "ctrl+f", " ":
		m.move(m.bodyHeight())
	case "home", "g":
		m.move(-1 << 20)
	case "end", "G":
		m.move(1 << 20)
	case "left", "h":
		return m.left()
	case "right", "l":
		return m.right()
	case "enter":
		if m.focus == focusList {
			return m.openSelected()
		}
		if e, ok := m.treeEntry(); ok {
			return m.activate(e)
		}
	}
	return nil
}

func (m *model) move(d int) {
	if m.focus == focusTree {
		m.treeCur += d
	} else {
		m.listCur += d
	}
	m.clamp()
}

func (m *model) left() tea.Cmd {
	if m.focus == focusList {
		m.focus = focusTree
		return nil
	}
	e, ok := m.treeEntry()
	if !ok {
		return nil
	}
	if e.expandable && e.expanded {
		m.tree.toggle(e)
		m.rebuild()
		return nil
	}
	for i := m.treeCur - 1; i >= 0; i-- {
		if m.rows[i].level < e.level {
			m.treeCur = i
			break
		}
	}
	m.clamp()
	return nil
}

func (m *model) right() tea.Cmd {
	if m.focus == focusList {
		return nil
	}
	e, ok := m.treeEntry()
	if !ok {
		return nil
	}
	if e.expandable && !e.expanded {
		return m.activate(e)
	}
	if _, isList := e.target(); isList {
		return m.activate(e)
	}
	return nil
}

// activate 处理左栏条目的选择：展开分组，或加载榜单并聚焦右栏。
func (m *model) activate(e entry) tea.Cmd {
	switch e.kind {
	case kindMenu, kindFollowing:
		need := m.tree.toggle(e)
		m.rebuild()
		if need {
			return m.loadMenu(e.menuID)
		}
		if e.kind == kindFollowing && m.tree.followOpen && len(m.tree.following) == 0 {
			m.flash("还没有关注的榜单，可在网站上添加关注", false)
		}
		return nil
	case kindTab:
		if e.expandable {
			if !e.expanded {
				m.tree.toggle(e)
				m.rebuild()
			}
			// 有二级 Tab 时默认打开第一个。
			for i := m.treeCur + 1; i < len(m.rows); i++ {
				c := m.rows[i]
				if c.kind == kindSubTab && c.tab == e.tab && c.menuID == e.menuID {
					m.treeCur = i
					m.clamp()
					t, _ := c.target()
					m.focus = focusList
					return m.loadList(t)
				}
			}
			return nil
		}
	}
	t, ok := e.target()
	if !ok {
		return nil
	}
	m.focus = focusList
	return m.loadList(t)
}

func (m *model) treeEntry() (entry, bool) {
	if m.treeCur < 0 || m.treeCur >= len(m.rows) {
		return entry{}, false
	}
	return m.rows[m.treeCur], true
}

func (m *model) selected() (row, bool) {
	if m.listCur < 0 || m.listCur >= len(m.items) {
		return row{}, false
	}
	return m.items[m.listCur], true
}

func (m *model) openSelected() tea.Cmd {
	it, ok := m.selected()
	if !ok || it.url == "" {
		return nil
	}
	if m.opt.Open == nil {
		return nil
	}
	if err := m.opt.Open(it.url); err != nil {
		m.flash("无法打开浏览器："+it.url, true)
		return nil
	}
	m.flash("已在浏览器中打开", false)
	return nil
}

func (m *model) copySelected() tea.Cmd {
	it, ok := m.selected()
	if !ok || it.url == "" {
		return nil
	}
	if m.opt.Copy == nil || m.opt.Copy(it.url) != nil {
		m.flash("无法访问剪贴板，链接："+it.url, true)
		return nil
	}
	m.flash("已复制链接", false)
	return nil
}

// ---- 鼠标 ----

func (m *model) onMouse(ev tea.MouseMsg) tea.Cmd {
	if m.help || m.searching {
		return nil
	}
	pane, ok := m.paneAt(ev.X)
	if ev.Action == tea.MouseActionMotion && ev.Button == tea.MouseButtonNone {
		m.setHover(pane, ok, ev.Y-1)
		return nil
	}
	if !ok {
		return nil
	}
	switch ev.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		if ev.Action != tea.MouseActionPress {
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
	y := ev.Y - 1 // 首行是标题栏
	if y < 0 || y >= m.bodyHeight() {
		return nil
	}
	now := m.opt.Now()
	if pane == focusTree {
		idx := m.treeOff + y
		if idx >= len(m.rows) {
			return nil
		}
		m.focus, m.treeCur = focusTree, idx
		m.lastClickAt, m.lastClickIdx, m.lastClickPan = now, idx, pane
		return m.activate(m.rows[idx])
	}
	idx := m.listOff + y
	if idx >= len(m.items) {
		return nil
	}
	double := m.lastClickPan == focusList && m.lastClickIdx == idx && now.Sub(m.lastClickAt) <= doubleClick
	m.focus, m.listCur = focusList, idx
	m.lastClickAt, m.lastClickIdx, m.lastClickPan = now, idx, pane
	if double {
		m.lastClickAt = time.Time{}
		return m.openSelected()
	}
	return nil
}

// setHover 记录鼠标悬停的行，y 是相对正文区域的行号。
func (m *model) setHover(pane focus, ok bool, y int) {
	m.hoverOK = false
	if !ok || y < 0 || y >= m.bodyHeight() {
		return
	}
	idx, n := m.listOff+y, len(m.items)
	if pane == focusTree {
		idx, n = m.treeOff+y, len(m.rows)
	}
	if idx < n {
		m.hoverOK, m.hoverPane, m.hoverIdx = true, pane, idx
	}
}

func (m *model) hovered(pane focus, idx int) bool {
	return m.hoverOK && m.hoverPane == pane && m.hoverIdx == idx
}

func (m *model) scroll(pane focus, d int) {
	m.hoverOK = false
	h := m.bodyHeight()
	if pane == focusTree {
		m.treeOff = clampOff(m.treeOff+d, len(m.rows), h)
		m.treeCur = clampInto(m.treeCur, m.treeOff, h, len(m.rows))
		return
	}
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
	h := m.bodyHeight()
	m.treeCur, m.treeOff = fit(m.treeCur, m.treeOff, len(m.rows), h)
	m.listCur, m.listOff = fit(m.listCur, m.listOff, len(m.items), h)
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
