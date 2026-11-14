package tui

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/cli"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/golden"
	"github.com/rebang-today/rebang-cli/internal/update"
)

type fakeSource struct {
	menus     []api.Menu
	data      map[int64]*menuData
	menuErr   error
	follow    []api.Tab
	followErr error
	lists     map[string][]cli.ListItem
	listErr   error
	listCalls []string
	search    []cli.SearchResultItem
	searchQ   string
	account   *cli.AccountView
	acctErr   error
	manifest  *update.Manifest
	installed string
	// AI 总结：shorts 为批量结果，insights 为详情，缺失表示暂无总结。
	shorts       map[string]string
	shortsErr    error
	shortsCalls  [][]string
	insights     map[string]*api.Insight
	insightErr   error
	insightCalls []string
	// 内容监控：monHits 是全部命中内容，按 cursor（偏移量）分页。
	monPlans     []api.MonitorPlan
	monUnread    *api.MonitorUnread
	monHits      []api.MonitorHit
	monErr       error
	monHitsErr   error
	monReadErr   error
	monHitsCalls []api.MonitorHitsQuery
	monReadCalls []string
	monUnreadN   int
}

func (f *fakeSource) MonitorPlans(context.Context) ([]api.MonitorPlan, error) {
	if f.monErr != nil {
		return nil, f.monErr
	}
	if f.monPlans == nil {
		return []api.MonitorPlan{}, nil
	}
	return f.monPlans, nil
}
func (f *fakeSource) MonitorUnread(context.Context) (*api.MonitorUnread, error) {
	f.monUnreadN++
	if f.monErr != nil {
		return nil, f.monErr
	}
	if f.monUnread == nil {
		return &api.MonitorUnread{}, nil
	}
	return f.monUnread, nil
}
func (f *fakeSource) MonitorHits(_ context.Context, q api.MonitorHitsQuery) (*api.MonitorHits, error) {
	f.monHitsCalls = append(f.monHitsCalls, q)
	if f.monHitsErr != nil {
		return nil, f.monHitsErr
	}
	var all []api.MonitorHit
	for _, h := range f.monHits {
		if q.UnreadOnly && h.IsRead {
			continue
		}
		if q.PlanID > 0 {
			hit := false
			for _, p := range h.PlanMatches {
				hit = hit || p.PlanID == q.PlanID
			}
			if !hit {
				continue
			}
		}
		all = append(all, h)
	}
	off, _ := strconv.Atoi(q.Cursor)
	end := min(off+q.PageSize, len(all))
	off = min(off, end)
	res := &api.MonitorHits{Items: all[off:end]}
	if end < len(all) {
		res.NextCursor, res.HasMore = strconv.Itoa(end), true
	}
	return res, nil
}
func (f *fakeSource) MonitorRead(_ context.Context, key string) (*api.MonitorUnread, error) {
	f.monReadCalls = append(f.monReadCalls, key)
	if f.monReadErr != nil {
		return nil, f.monReadErr
	}
	for i := range f.monHits {
		if f.monHits[i].ItemKey == key {
			f.monHits[i].IsRead = true
		}
	}
	if f.monUnread != nil {
		f.monUnread.All--
		for i := range f.monUnread.Plans {
			f.monUnread.Plans[i].UnreadCount--
		}
	}
	return f.monUnread, nil
}

func (f *fakeSource) Menus(context.Context) ([]api.Menu, error) { return f.menus, nil }
func (f *fakeSource) MenuData(_ context.Context, id int64) (*menuData, error) {
	if id == followMenuID {
		if f.followErr != nil {
			return nil, f.followErr
		}
		return &menuData{tabs: f.follow}, nil
	}
	if f.menuErr != nil {
		return nil, f.menuErr
	}
	return f.data[id], nil
}
func (f *fakeSource) List(_ context.Context, t listTarget, _ int) (*cli.ListResult, error) {
	f.listCalls = append(f.listCalls, t.key())
	if f.listErr != nil {
		return &cli.ListResult{}, f.listErr
	}
	return &cli.ListResult{Items: f.lists[t.key()]}, nil
}
func (f *fakeSource) Search(_ context.Context, q string, _ int) (*cli.SearchResult, error) {
	f.searchQ = q
	return &cli.SearchResult{Query: q, Items: f.search}, nil
}
func (f *fakeSource) Account(context.Context) (*cli.AccountView, error) { return f.account, f.acctErr }
func (f *fakeSource) InsightShorts(_ context.Context, keys []string) (map[string]string, error) {
	f.shortsCalls = append(f.shortsCalls, keys)
	if f.shortsErr != nil {
		return nil, f.shortsErr
	}
	out := map[string]string{}
	for _, k := range keys {
		if v, ok := f.shorts[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}
func (f *fakeSource) Insight(_ context.Context, key string) (*api.Insight, error) {
	f.insightCalls = append(f.insightCalls, key)
	if f.insightErr != nil {
		return nil, f.insightErr
	}
	if in, ok := f.insights[key]; ok {
		return in, nil
	}
	return nil, errNoSummary
}
func (f *fakeSource) CheckUpdate(context.Context) (*update.Manifest, error) {
	return f.manifest, nil
}
func (f *fakeSource) Install(_ context.Context, m *update.Manifest) error {
	f.installed = m.Version
	return nil
}

func newFake() *fakeSource {
	return &fakeSource{
		menus: []api.Menu{{ID: 1, Name: "综合"}, {ID: 2, Name: "科技"}},
		data: map[int64]*menuData{
			1: {tabs: []api.Tab{
				{Key: "zhihu", Name: "知乎"},
				{Key: "weibo", Name: "微博", Children: []api.SubTab{{Key: "hot", Name: "热搜"}, {Key: "news", Name: "要闻"}}},
			}},
			2: {nodes: []api.Node{{NodeID: "n1", Name: "少数派"}}},
		},
		lists: map[string][]cli.ListItem{
			"|zhihu|":     {{Title: "知乎第一", URL: "https://example.com/z1", Heat: "100万", ItemKey: "kz1"}, {Title: "知乎第二", URL: "https://example.com/z2", ItemKey: "kz2"}},
			"|weibo|hot":  {{Title: "微博热搜", URL: "https://example.com/w1"}},
			"n1||":        {{Title: "少数派文章", URL: "https://example.com/s1"}},
			"|weibo|news": {{Title: "微博要闻", URL: "https://example.com/w2"}},
		},
	}
}

// drain 同步执行命令并把产生的消息送回模型，忽略长时间阻塞的命令（定时器、光标闪烁）。
func drain(t *testing.T, m *model, cmd tea.Cmd) {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 200 {
			t.Fatal("命令过多，可能存在循环")
		}
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		ch := make(chan tea.Msg, 1)
		go func() { ch <- c() }()
		var msg tea.Msg
		select {
		case msg = <-ch:
		case <-time.After(100 * time.Millisecond):
			continue
		}
		switch msg := msg.(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case tea.QuitMsg:
		default:
			_, next := m.Update(msg)
			queue = append(queue, next)
		}
	}
}

func start(t *testing.T, f *fakeSource, opt Options) *model {
	t.Helper()
	opt.Source = f
	if opt.Version == "" {
		opt.Version = "0.1.0"
	}
	m := newModel(opt)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	drain(t, m, m.Init())
	return m
}

func key(t *testing.T, m *model, keys ...string) {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "shift+tab":
			msg = tea.KeyMsg{Type: tea.KeyShiftTab}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
		case "shift+up":
			msg = tea.KeyMsg{Type: tea.KeyShiftUp}
		case "shift+down":
			msg = tea.KeyMsg{Type: tea.KeyShiftDown}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		_, cmd := m.Update(msg)
		drain(t, m, cmd)
	}
}

func TestStartupLoadsFirstList(t *testing.T) {
	f := newFake()
	m := start(t, f, Options{})
	if m.target.tab != "zhihu" || len(m.items) != 2 {
		t.Fatalf("启动后应打开第一个榜单，target=%+v items=%d", m.target, len(m.items))
	}
	if m.focus != focusTree || m.srcCur != 0 || m.menuIdx != 1 {
		t.Fatalf("启动后焦点应在左栏的第一个榜单，focus=%v cur=%d", m.focus, m.srcCur)
	}
	v := m.View()
	for _, want := range []string{"0 订阅", "1 综合", "2 科技", "知乎第一", "100万", "2 条", "未登录", "/ 搜索"} {
		if !strings.Contains(v, want) {
			t.Errorf("界面缺少 %q\n%s", want, v)
		}
	}
	if got := len(strings.Split(v, "\n")); got != 20 {
		t.Errorf("界面行数 = %d, 想要 20", got)
	}
}

// settle 模拟 debounce 到期。
func settle(t *testing.T, m *model) {
	t.Helper()
	if !m.debPending {
		t.Fatal("应有等待中的延迟加载")
	}
	_, cmd := m.Update(debounceMsg{m.debSeq})
	drain(t, m, cmd)
}

func TestLeftMoveDebouncedLoad(t *testing.T) {
	f := newFake()
	m := start(t, f, Options{})
	n := len(f.listCalls)
	key(t, m, "j") // 微博
	if len(f.listCalls) != n || m.focus != focusTree || len(m.items) != 0 {
		t.Fatalf("移动后应先等待，calls=%v focus=%v items=%d", f.listCalls, m.focus, len(m.items))
	}
	if v := m.View(); !strings.Contains(v, "微博") || !strings.Contains(v, "热搜") || !strings.Contains(v, "要闻") {
		t.Fatalf("应立即显示新的标题与子榜单条\n%s", v)
	}
	// 过期的延迟不触发加载。
	_, cmd := m.Update(debounceMsg{m.debSeq - 1})
	drain(t, m, cmd)
	if len(f.listCalls) != n {
		t.Fatal("过期的延迟不应加载")
	}
	settle(t, m)
	if m.target.tab != "weibo" || m.target.subTab != "hot" || m.items[0].title != "微博热搜" {
		t.Fatalf("应加载微博的第一个子榜单，target=%+v items=%+v", m.target, m.items)
	}
	if m.focus != focusTree {
		t.Fatal("焦点应留在左栏")
	}
	// 快速连续移动只加载最后停下的榜单。
	key(t, m, "k", "j")
	n = len(f.listCalls)
	settle(t, m)
	if len(f.listCalls) != n+1 || f.listCalls[n] != "|weibo|hot" {
		t.Fatalf("应只加载一次，calls=%v", f.listCalls[n:])
	}
	// 回车进入列表，再回车打开。
	var opened string
	m.opt.Open = func(u string) error { opened = u; return nil }
	key(t, m, "enter")
	if m.focus != focusList {
		t.Fatal("左栏回车应进入列表")
	}
	key(t, m, "enter")
	if opened != "https://example.com/w1" {
		t.Fatalf("列表回车应打开链接，opened=%q", opened)
	}
	key(t, m, "esc")
	if m.focus != focusList {
		t.Fatal("列表中 Esc 不应切到左栏")
	}
	key(t, m, "right", "l")
	if m.focus != focusList || m.listCur != 0 {
		t.Fatal("榜单中 → 不做任何事")
	}
	key(t, m, "left")
	if m.focus != focusTree {
		t.Fatal("← 应回到左栏")
	}
	n = len(f.listCalls)
	key(t, m, "h", "left")
	if m.focus != focusTree || len(f.listCalls) != n {
		t.Fatal("左栏中 ← 不做任何事")
	}
	key(t, m, "l")
	if m.focus != focusList || len(f.listCalls) != n {
		t.Fatal("→ 应回到榜单；没有等待中的加载时不请求")
	}
	// 左栏移动后马上按 →：立即加载，不等防抖。
	key(t, m, "h", "k", "right")
	if m.focus != focusList || m.debPending || m.target.tab != "zhihu" || len(f.listCalls) != n+1 {
		t.Fatalf("→ 应立即加载：target=%+v calls=%v", m.target, f.listCalls[n:])
	}
}

func TestEnterSkipsDebounce(t *testing.T) {
	f := newFake()
	m := start(t, f, Options{})
	key(t, m, "j", "enter")
	if m.debPending || m.target.subTab != "hot" || len(m.items) != 1 || m.focus != focusList {
		t.Fatalf("回车应立即加载并进入列表，target=%+v", m.target)
	}
}

func TestSubTabSwitch(t *testing.T) {
	f := newFake()
	m := start(t, f, Options{})
	n := len(f.listCalls)
	key(t, m, "tab", "shift+tab")
	if m.target.tab != "zhihu" || m.subIdx != 0 || len(f.listCalls) != n {
		t.Fatal("没有子榜单时 tab 不应有变化")
	}
	key(t, m, "j", "enter", "tab")
	if m.target.subTab != "news" || m.items[0].title != "微博要闻" || m.focus != focusList {
		t.Fatalf("榜单中 tab 应切换子榜单，target=%+v", m.target)
	}
	key(t, m, "tab")
	if m.target.subTab != "hot" {
		t.Fatal("最后一个子榜单再按 tab 应回到第一个")
	}
	key(t, m, "shift+tab")
	if m.target.subTab != "news" {
		t.Fatal("第一个子榜单按 shift+tab 应到最后一个")
	}
	key(t, m, "left", "shift+tab")
	if m.target.subTab != "hot" || m.focus != focusTree {
		t.Fatalf("左栏中 tab 也应切换子榜单，target=%+v", m.target)
	}
	// 不再用 ←→ 切换子榜单。
	key(t, m, "right", "right", "left")
	if m.target.subTab != "hot" {
		t.Fatalf("←→ 不应切换子榜单：%+v", m.target)
	}
	key(t, m, "tab")
	// 子榜单按来源记住。
	key(t, m, "k")
	settle(t, m)
	key(t, m, "j")
	settle(t, m)
	if m.target.subTab != "news" {
		t.Fatalf("回到微博应恢复上次的子榜单，target=%+v", m.target)
	}
}

func TestMenuSwitchRestoresPosition(t *testing.T) {
	f := newFake()
	m := start(t, f, Options{})
	key(t, m, "j", "tab") // 切换子榜单立即加载，取消等待中的延迟
	if m.debPending || m.target.subTab != "news" {
		t.Fatalf("target=%+v", m.target)
	}
	key(t, m, "2")
	if m.menuIdx != 2 || m.target.node != "n1" || m.items[0].title != "少数派文章" {
		t.Fatalf("2 应切换到科技并打开第一个节点，target=%+v", m.target)
	}
	if m.focus != focusTree || m.hasStrip() {
		t.Fatal("切换菜单不改变焦点；节点没有子榜单条")
	}
	key(t, m, "1")
	if m.srcCur != 1 || m.target.tab != "weibo" || m.target.subTab != "news" || m.items[0].title != "微博要闻" {
		t.Fatalf("回到综合应恢复微博 · 要闻，cur=%d target=%+v", m.srcCur, m.target)
	}
	key(t, m, "]")
	if m.menuIdx != 2 {
		t.Fatal("] 应切换到下一个分类")
	}
	key(t, m, "]")
	if m.menuIdx != 0 {
		t.Fatal("] 在最后一个分类应回到订阅")
	}
	key(t, m, "[")
	if m.menuIdx != 2 {
		t.Fatal("[ 应循环到最后一个分类")
	}
	key(t, m, "9")
	if m.menuIdx != 2 {
		t.Fatal("不存在的数字键应忽略")
	}
}

func TestMenuLoadErrorAndRetry(t *testing.T) {
	f := newFake()
	f.menus = append(f.menus, api.Menu{ID: 3, Name: "新闻"})
	m := start(t, f, Options{})
	f.menuErr = errs.Network(errors.New("dial tcp: refused"))
	key(t, m, "3")
	if v := m.View(); !strings.Contains(v, "加载失败") || !strings.Contains(v, "r 重试") {
		t.Fatalf("菜单加载失败应提示重试\n%s", v)
	}
	f.menuErr = nil
	f.data[3] = &menuData{nodes: []api.Node{{NodeID: "n1", Name: "少数派"}}}
	key(t, m, "r")
	if m.target.node != "n1" || len(m.items) != 1 {
		t.Fatalf("r 应重新加载菜单，target=%+v", m.target)
	}
}

func TestEmptyListHint(t *testing.T) {
	f := newFake()
	f.lists["|zhihu|"] = nil
	m := start(t, f, Options{})
	v := m.View()
	if !strings.Contains(v, "暂无内容") || !strings.Contains(v, "↓ 换一个榜单") {
		t.Fatalf("左栏焦点提示 ↓ 换一个榜单\n%s", v)
	}
	// 榜单焦点下 ↓ 不换来源，提示 ←。
	key(t, m, "l")
	if v := plainView(m); !strings.Contains(v, "← 换一个榜单") {
		t.Fatalf("榜单焦点提示 ← 换一个榜单\n%s", v)
	}
	// 单栏启动时焦点在榜单。
	m2 := start(t, f, Options{})
	m2.Update(tea.WindowSizeMsg{Width: 48, Height: 12})
	if v := plainView(m2); !strings.Contains(v, "← 换一个榜单") {
		t.Fatalf("单栏提示 ← 换一个榜单\n%s", v)
	}
}

func TestOpenCopyAndRefresh(t *testing.T) {
	f := newFake()
	var opened, copied string
	m := start(t, f, Options{
		Open: func(u string) error { opened = u; return nil },
		Copy: func(u string) error { copied = u; return nil },
	})
	key(t, m, "l", "j", "enter")
	if opened != "https://example.com/z2" {
		t.Fatalf("打开的链接 = %q", opened)
	}
	key(t, m, "y")
	if copied != "https://example.com/z2" || !strings.Contains(m.View(), "已复制链接") {
		t.Fatalf("复制的链接 = %q", copied)
	}
	n := len(f.listCalls)
	key(t, m, "r")
	if len(f.listCalls) != n+1 || m.listCur != 1 {
		t.Fatalf("r 应刷新当前榜单并保持光标，calls=%v cur=%d", f.listCalls, m.listCur)
	}
	m.opt.Copy = func(string) error { return errors.New("no clipboard") }
	key(t, m, "y")
	if !strings.Contains(m.message, "https://example.com/z2") {
		t.Fatalf("剪贴板不可用时应显示链接，message=%q", m.message)
	}
}

func TestSearchAndBack(t *testing.T) {
	f := newFake()
	f.search = []cli.SearchResultItem{{Title: "搜索结果", URL: "https://example.com/r", SourceName: "知乎"}}
	m := start(t, f, Options{})
	key(t, m, "/", "q", "a", "i", "enter")
	if f.searchQ != "qai" || m.searchQ != "qai" {
		t.Fatalf("搜索词 = %q", f.searchQ)
	}
	if v := m.View(); !strings.Contains(v, "搜索结果") || !strings.Contains(v, "搜索“qai”") {
		t.Fatalf("应显示搜索结果\n%s", v)
	}
	key(t, m, "esc")
	if m.searchQ != "" || m.items[0].title != "知乎第一" {
		t.Fatal("Esc 应回到之前的榜单")
	}
	key(t, m, "/", "x", "esc")
	if m.searching || f.searchQ != "qai" {
		t.Fatal("搜索框中 Esc 应取消")
	}
}

func TestExpiredLoginAndMembershipError(t *testing.T) {
	f := newFake()
	f.acctErr = errs.Unauthorized("登录已失效")
	m := start(t, f, Options{LoggedIn: true})
	if !m.expired || !strings.Contains(m.View(), "登录已失效") {
		t.Fatal("401 时应提示登录失效")
	}

	f = newFake()
	f.listErr = errs.Membership("该榜单仅对会员开放")
	m = start(t, f, Options{})
	if !strings.Contains(m.View(), "该榜单仅对会员开放") {
		t.Fatalf("应提示需要会员\n%s", m.View())
	}
}

func click(t *testing.T, m *model, x, y int) {
	t.Helper()
	_, cmd := m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	drain(t, m, cmd)
}

func TestMouse(t *testing.T) {
	f := newFake()
	var opened string
	now := time.Unix(1700000000, 0)
	m := start(t, f, Options{Open: func(u string) error { opened = u; return nil }, Now: func() time.Time { return now }})
	x := m.rightX() + 5
	// 右栏：第 1 行标题，列表从第 2 行开始；第二条在 y=3。
	click(t, m, x, 3)
	if m.listCur != 1 || opened != "" || m.focus != focusList {
		t.Fatalf("单击应选中，cur=%d opened=%q", m.listCur, opened)
	}
	now = now.Add(200 * time.Millisecond)
	click(t, m, x, 3)
	if opened != "https://example.com/z2" {
		t.Fatalf("双击应打开，opened=%q", opened)
	}
	click(t, m, 2, 2) // 左栏第 2 项：微博
	if m.target.tab != "weibo" || m.items[0].title != "微博热搜" || m.focus != focusTree || m.debPending {
		t.Fatalf("点击左栏应立即打开榜单且焦点在左栏，target=%+v focus=%v", m.target, m.focus)
	}
	// 子榜单条在 y=2。
	var newsX int
	for _, sp := range m.stripSpans(m.rightX()) {
		if sp.idx == 1 {
			newsX = sp.x0 + 1
		}
	}
	if newsX == 0 {
		t.Fatal("找不到子榜单“要闻”")
	}
	click(t, m, newsX, 2)
	if m.target.subTab != "news" || m.focus != focusTree {
		t.Fatalf("点击子榜单应切换且不改变焦点，target=%+v focus=%v", m.target, m.focus)
	}
	// 标题栏的菜单。
	var menuX int
	for _, sp := range m.menuSpans() {
		if sp.idx == 2 {
			menuX = sp.x0 + 1
		}
	}
	click(t, m, menuX, 0)
	if m.menuIdx != 2 || m.target.node != "n1" || m.focus != focusTree {
		t.Fatalf("点击菜单应切换分类，menu=%d target=%+v", m.menuIdx, m.target)
	}
	_, _ = m.Update(tea.MouseMsg{X: 2, Y: 3, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
}

func TestNarrowLayout(t *testing.T) {
	f := newFake()
	f.lists["|zhihu|"][0].Heat = "1234万热度"
	f.account = &cli.AccountView{ID: 1, Email: "a-very-long-address@example.com", IsMember: true, MemberExpiresAt: "2027-01-01T00:00:00+08:00"}
	f.lists["|zhihu|"][1].Title = "知乎第二：一个很长很长的标题，单栏放不下热度时应该优先显示完整标题"
	m := newModel(Options{Source: f, Version: "0.1.0", LoggedIn: true})
	m.Update(tea.WindowSizeMsg{Width: 48, Height: 12})
	drain(t, m, m.Init())
	v := m.View()
	if strings.Contains(v, "│") || !strings.Contains(v, "知乎第一") || strings.Contains(v, "微博") || m.focus != focusList {
		t.Fatalf("48 列单栏启动时应显示榜单\n%s", v)
	}
	if st := golden.StripANSI(m.statusLine()); !strings.Contains(st, "? 帮助  q 退出") {
		t.Fatalf("单栏底部提示应保留 ? 与 q\n%s", v)
	}
	if strings.Contains(v, "1234万") {
		t.Fatalf("单栏放不下热度时应隐藏热度列\n%s", v)
	}
	key(t, m, "j")
	if m.listCur != 1 {
		t.Fatal("↑↓ 应在榜单里移动")
	}

	// ← 打开整栏的来源列表；↑↓ 实时切换来源（防抖）；→ 回到榜单。
	key(t, m, "left")
	if v := m.View(); !strings.Contains(v, "微博") || strings.Contains(v, "知乎第一") || !m.picking() {
		t.Fatalf("← 应打开来源列表\n%s", v)
	}
	if st := golden.StripANSI(m.statusLine()); !strings.Contains(st, "→ 进入榜单") {
		t.Fatalf("来源列表的提示：%s", st)
	}
	calls := len(f.listCalls)
	key(t, m, "j")
	if !m.debPending || m.target.tab != "weibo" {
		t.Fatalf("↑↓ 应实时切换来源，target=%+v", m.target)
	}
	key(t, m, "right")
	if m.picking() || m.debPending || m.target.tab != "weibo" || m.items[0].title != "微博热搜" || len(f.listCalls) != calls+1 {
		t.Fatalf("→ 应回到榜单并立即加载，target=%+v", m.target)
	}
	key(t, m, "left", "k")
	settle(t, m)
	key(t, m, "esc")
	if !m.picking() || m.target.tab != "zhihu" {
		t.Fatal("Esc 不关闭来源列表，已切换的来源不恢复")
	}
	// 回车同样回到榜单。
	key(t, m, "j", "enter")
	if m.picking() || m.target.tab != "weibo" || m.items[0].title != "微博热搜" {
		t.Fatalf("回车应回到榜单，target=%+v", m.target)
	}
	v = m.View()
	if !strings.Contains(v, "微博") || !strings.Contains(v, "热搜") || !strings.Contains(v, "要闻") || !strings.Contains(v, "微博热搜") {
		t.Fatalf("标题行应显示来源名，子榜单条照常显示\n%s", v)
	}
	key(t, m, "tab")
	if m.target.subTab != "news" {
		t.Fatalf("tab 应切换子榜单，target=%+v", m.target)
	}
	key(t, m, "2")
	if m.menuIdx != 2 || m.picking() || m.target.node != "n1" {
		t.Fatalf("数字键应切换分类，menu=%d target=%+v", m.menuIdx, m.target)
	}

	// 来源列表里切换分类：留在来源列表。
	key(t, m, "left", "1")
	if !m.picking() || m.menuIdx != 1 {
		t.Fatalf("来源列表中按数字键应切换分类，menu=%d", m.menuIdx)
	}
	key(t, m, "right")

	// 56 列及以上仍为双栏。
	m.Update(tea.WindowSizeMsg{Width: 56, Height: 12})
	if v := m.View(); !strings.Contains(v, "│") || !strings.Contains(v, "知乎") {
		t.Fatalf("56 列应为双栏\n%s", v)
	}
}

// TestTruncation 窄屏下菜单栏与子榜单条以当前项为中心截断，不超出宽度。
func TestTruncation(t *testing.T) {
	f := newFake()
	f.menus = nil
	for i := int64(1); i <= 8; i++ {
		f.menus = append(f.menus, api.Menu{ID: i, Name: []string{"综合", "科技", "新闻", "娱乐", "社区", "开发", "财经", "购物"}[i-1]})
		if i > 2 {
			f.data[i] = f.data[1]
		}
	}
	var kids []api.SubTab
	for _, n := range []string{"日榜", "周榜", "月榜", "热议", "新帖", "精华", "问答", "专栏", "视频", "圆桌"} {
		kids = append(kids, api.SubTab{Key: n, Name: n})
	}
	f.data[1].tabs[1].Children = kids
	m := start(t, f, Options{Now: fixedNow})
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	key(t, m, "4", "j")
	settle(t, m)
	for i := 0; i < 6; i++ {
		key(t, m, "tab")
	}
	key(t, m, "l")
	v := m.View()
	for i, line := range strings.Split(v, "\n") {
		if w := lipgloss.Width(line); w != 60 {
			t.Errorf("第 %d 行宽 %d：%q", i, w, golden.StripANSI(line))
		}
	}
	plain := golden.StripANSI(v)
	if !strings.Contains(plain, "4 娱乐") || strings.Contains(plain, "1 综合") || !strings.Contains(plain, "‹") || !strings.Contains(plain, "›") {
		t.Fatalf("菜单栏应以当前项为中心截断\n%s", plain)
	}
	if !strings.Contains(plain, "专栏") || strings.Contains(plain, "日榜") {
		t.Fatalf("子榜单条应以当前项为中心截断\n%s", plain)
	}
	// 截断后的点击位置仍然正确。
	for _, sp := range m.menuSpans() {
		if sp.idx == 5 {
			click(t, m, sp.x0, 0)
		}
	}
	if m.menuIdx != 5 {
		t.Fatalf("点击截断后的菜单栏，menu=%d", m.menuIdx)
	}
}

func TestHelpAndQuit(t *testing.T) {
	m := start(t, newFake(), Options{})
	key(t, m, "?")
	if !strings.Contains(m.View(), "快捷键") {
		t.Fatal("? 应显示帮助")
	}
	key(t, m, "j")
	if m.help {
		t.Fatal("任意键应关闭帮助")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q 应退出")
	}
}

func TestUpdateNoticeAndAutoUpdate(t *testing.T) {
	f := newFake()
	f.manifest = &update.Manifest{Version: "0.2.0"}
	m := start(t, f, Options{})
	if !strings.Contains(m.View(), "0.2.0") || f.installed != "" {
		t.Fatal("有新版本时应提示且默认不自动安装")
	}
	f = newFake()
	f.manifest = &update.Manifest{Version: "0.2.0", MinVersion: "0.2.0"}
	m = start(t, f, Options{AutoUpdate: true})
	if f.installed != "0.2.0" || !strings.Contains(m.message, "已升级到 0.2.0") {
		t.Fatalf("auto_update 开启时应自动安装，message=%q", m.message)
	}
}

func TestRefreshTick(t *testing.T) {
	f := newFake()
	m := start(t, f, Options{Refresh: time.Hour})
	n := len(f.listCalls)
	_, cmd := m.Update(tickMsg{})
	drain(t, m, cmd)
	if len(f.listCalls) != n+1 {
		t.Fatalf("定时刷新应重新加载榜单，calls=%v", f.listCalls)
	}
}

func TestTwoPaneThreshold(t *testing.T) {
	m := start(t, newFake(), Options{})
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	v := m.View()
	if !m.wide() || !strings.Contains(v, "│") || !strings.Contains(v, "微博") || !strings.Contains(v, "知乎第一") {
		t.Fatalf("60 列应为双栏\n%s", v)
	}
	key(t, m, "j")
	settle(t, m)
	if v := m.View(); !strings.Contains(v, "微博热搜") || m.focus != focusTree {
		t.Fatalf("60 列左栏换榜单后应直接看到列表\n%s", v)
	}
	m.Update(tea.WindowSizeMsg{Width: wideMinWidth - 1, Height: 10})
	if v := m.View(); m.wide() || strings.Contains(v, "│") {
		t.Fatalf("阈值以下应为单栏\n%s", v)
	}
}

// wechatFake 在综合下加一个公众号来源（关注、热门两个子榜单）。
func wechatFake() *fakeSource {
	f := newFake()
	f.data[1].tabs = append(f.data[1].tabs, api.Tab{Key: "wechat", Name: "公众号", Children: []api.SubTab{{Key: "following", Name: "关注"}, {Key: "hot", Name: "热门"}}})
	f.lists["|wechat|hot"] = []cli.ListItem{{Title: "公众号热门", URL: "https://example.com/wx1"}}
	return f
}

func TestWechatDefaultsToHotWhenLoggedOut(t *testing.T) {
	f := wechatFake()
	m := start(t, f, Options{})
	key(t, m, "G")
	settle(t, m)
	if m.target.subTab != "hot" || m.items[0].title != "公众号热门" {
		t.Fatalf("未登录进入公众号应选热门，target=%+v", m.target)
	}
	for _, c := range f.listCalls {
		if c == "|wechat|following" {
			t.Fatal("未登录时不应请求关注子榜")
		}
	}
	// 手动切到关注不会被改回。
	key(t, m, "tab")
	if m.target.subTab != "following" || len(m.items) != 0 || !strings.Contains(m.View(), "暂无内容") {
		t.Fatalf("手动选中的关注应照常显示，target=%+v", m.target)
	}
}

func TestWechatFallsBackWhenFollowingEmpty(t *testing.T) {
	f := wechatFake()
	m := start(t, f, Options{LoggedIn: true})
	key(t, m, "G")
	settle(t, m)
	n := len(f.listCalls)
	if n < 2 || f.listCalls[n-2] != "|wechat|following" || f.listCalls[n-1] != "|wechat|hot" {
		t.Fatalf("已登录应先加载关注，为空再切到热门，calls=%v", f.listCalls)
	}
	if m.target.subTab != "hot" || m.items[0].title != "公众号热门" {
		t.Fatalf("target=%+v", m.target)
	}
	// 记为该来源的子榜：离开再回来直接是热门。
	key(t, m, "k")
	settle(t, m)
	key(t, m, "j")
	settle(t, m)
	if f.listCalls[len(f.listCalls)-1] != "|wechat|hot" || m.target.subTab != "hot" {
		t.Fatalf("应记住热门，calls=%v", f.listCalls)
	}
	// 手动切到关注，即使为空也不改回。
	key(t, m, "shift+tab")
	if m.target.subTab != "following" || len(m.items) != 0 {
		t.Fatalf("手动选中的关注不应被改回，target=%+v", m.target)
	}
	key(t, m, "k")
	settle(t, m)
	key(t, m, "j")
	settle(t, m)
	if m.target.subTab != "following" {
		t.Fatalf("应记住手动选的关注，target=%+v", m.target)
	}

	// 已登录且关注不为空时停在关注。
	f2 := wechatFake()
	f2.lists["|wechat|following"] = []cli.ListItem{{Title: "关注的文章", URL: "https://example.com/wx2"}}
	m = start(t, f2, Options{LoggedIn: true})
	key(t, m, "G")
	settle(t, m)
	if m.target.subTab != "following" || m.items[0].title != "关注的文章" {
		t.Fatalf("关注不为空时应停在关注，target=%+v", m.target)
	}
}

func TestSingleSubTabHidden(t *testing.T) {
	f := newFake()
	f.data[1].tabs[0].Children = []api.SubTab{{Key: "only", Name: "唯一"}}
	f.lists["|zhihu|only"] = f.lists["|zhihu|"]
	m := start(t, f, Options{})
	if m.target.subTab != "only" || len(m.items) != 2 {
		t.Fatalf("请求时应带上唯一子榜单的 key，target=%+v", m.target)
	}
	v := m.View()
	if m.hasStrip() || strings.Contains(v, "唯一") || strings.Contains(v, "tab 子榜") {
		t.Fatalf("只有一个子榜单时不显示子榜单条与 tab 提示\n%s", v)
	}
	n := len(f.listCalls)
	key(t, m, "tab", "shift+tab", "l", "tab")
	if len(f.listCalls) != n || m.target.subTab != "only" {
		t.Fatalf("tab 不应有任何动作，calls=%v", f.listCalls[n:])
	}
	if v := m.View(); strings.Contains(v, "tab 子榜") {
		t.Fatalf("榜单焦点也不显示 tab 提示\n%s", v)
	}
}

// TestInvisibleCharsStripped 标题中的不可见字符（反复制水印）不应输出到终端，也不能撑破行宽。
func TestInvisibleCharsStripped(t *testing.T) {
	junk := strings.Repeat("\u200b\u200c\u200d\u2061\u2062\u2063\u2064\u202c\ufeff\u00ad", 6)
	f := newFake()
	f.data[1].tabs[0].Name = junk + "知乎"
	f.lists["|zhihu|"] = []cli.ListItem{
		{Title: "普通标题", URL: "https://example.com/1", Heat: "1万"},
		{Title: junk + "带水印的标题" + junk + "\n第二行\t结尾", URL: "https://example.com/2", Heat: junk + "2万"},
	}
	m := start(t, f, Options{})
	for _, w := range []int{100, 60, 40} {
		m.Update(tea.WindowSizeMsg{Width: w, Height: 10})
		v := m.View()
		for i, line := range strings.Split(v, "\n") {
			if lw := lipgloss.Width(line); lw > w {
				t.Errorf("宽 %d：第 %d 行宽 %d", w, i, lw)
			}
		}
		for _, r := range v {
			if (r >= 0x200b && r <= 0x200f) || (r >= 0x202a && r <= 0x202e) || (r >= 0x2060 && r <= 0x2069) || r == 0xfeff || r == 0xad || r == '\t' {
				t.Fatalf("宽 %d：输出含 %U", w, r)
			}
		}
	}
	if m.items[1].title != "带水印的标题 第二行 结尾" || m.items[1].url != "https://example.com/2" {
		t.Fatalf("title=%q", m.items[1].title)
	}
}

func TestFollowMenu(t *testing.T) {
	f := newFake()
	f.follow = []api.Tab{
		{Key: "weibo", Name: "微博", Children: []api.SubTab{{Key: "hot", Name: "热搜"}, {Key: "news", Name: "要闻"}}},
		{Key: "monitoring", Name: "内容监控", Type: "monitoring", Children: []api.SubTab{{Key: "monitoring:all", Name: "全部"}}},
		{Key: "zhihu", Name: "知乎"},
	}
	m := start(t, f, Options{LoggedIn: true})
	if m.menuIdx != 1 || m.target.tab != "zhihu" {
		t.Fatal("启动时应停在综合")
	}
	key(t, m, "0")
	if m.menuIdx != 0 || m.target.tab != "weibo" || m.target.subTab != "hot" || m.items[0].title != "微博热搜" {
		t.Fatalf("0 应切到订阅并打开第一个订阅，target=%+v", m.target)
	}
	if v := m.View(); !strings.Contains(v, "0 订阅") || !strings.Contains(v, "监控") || !strings.Contains(v, "要闻") {
		t.Fatalf("订阅菜单应列出订阅的 Tab 与子榜单\n%s", v)
	}
	key(t, m, "tab")
	if m.target.subTab != "news" || m.focus != focusTree {
		t.Fatalf("tab 应切换子榜单，target=%+v", m.target)
	}
	// 内容监控没有计划：提示去网站创建，不崩溃。
	key(t, m, "j")
	settle(t, m)
	if m.loading || m.listErr != "" || !strings.HasPrefix(m.listNotice, "还没有监控计划") {
		t.Fatalf("内容监控没有计划时应提示，target=%+v err=%q notice=%q", m.target, m.listErr, m.listNotice)
	}
	if v := m.View(); strings.Contains(v, "tab 子榜") {
		t.Fatalf("没有计划时不显示子榜单\n%s", v)
	}
	key(t, m, "r", "tab")
	if !strings.HasPrefix(m.listNotice, "还没有监控计划") || m.listErr != "" {
		t.Fatalf("在内容监控上按键不应出错，err=%q", m.listErr)
	}
	key(t, m, "j")
	settle(t, m)
	if m.target.tab != "zhihu" || len(m.items) == 0 || m.listNotice != "" {
		t.Fatalf("↓ 应延迟加载下一个订阅，target=%+v", m.target)
	}
	key(t, m, "1", "0")
	if m.target.tab != "zhihu" {
		t.Fatalf("回到订阅应恢复上次的位置，target=%+v", m.target)
	}
	key(t, m, "k", "k")
	settle(t, m)
	if m.target.subTab != "news" {
		t.Fatalf("应记住订阅里微博的子榜单，target=%+v", m.target)
	}
}

func TestFollowMenuEmptyStates(t *testing.T) {
	// 未登录：左栏为空，右栏提示登录，不发请求。
	f := newFake()
	f.followErr = errors.New("不应请求订阅")
	m := start(t, f, Options{})
	key(t, m, "0")
	v := m.View()
	if m.menuIdx != 0 || len(m.curSources()) != 0 || !strings.Contains(v, "登录后查看订阅：rebang auth login") {
		t.Fatalf("未登录时应提示登录\n%s", v)
	}
	if strings.Contains(v, "不应请求订阅") {
		t.Fatal("未登录时不应请求订阅")
	}

	// 已登录但没有订阅。
	f = newFake()
	m = start(t, f, Options{LoggedIn: true})
	key(t, m, "0")
	if v := m.View(); !strings.Contains(v, "还没有订阅任何榜单") || !strings.Contains(v, "添加订阅") {
		t.Fatalf("没有订阅时应提示去网站添加\n%s", v)
	}

	// 接口报错：显示错误，r 重试。
	f = newFake()
	f.followErr = errs.Network(errors.New("dial tcp: refused"))
	m = start(t, f, Options{LoggedIn: true})
	key(t, m, "0")
	if v := m.View(); !strings.Contains(v, "加载失败") || !strings.Contains(v, "r 重试") {
		t.Fatalf("订阅加载失败应提示重试\n%s", v)
	}
	f.followErr = nil
	f.follow = []api.Tab{{Key: "zhihu", Name: "知乎"}}
	key(t, m, "r")
	if m.target.tab != "zhihu" || len(m.items) != 2 {
		t.Fatalf("r 应重新加载订阅，target=%+v", m.target)
	}
}

func TestLoggedInShowsMembership(t *testing.T) {
	f := newFake()
	f.account = &cli.AccountView{ID: 1, Email: "u@example.com", IsMember: true, MemberExpiresAt: "2027-01-01T00:00:00+08:00"}
	m := start(t, f, Options{LoggedIn: true})
	if v := m.View(); !strings.Contains(v, "u@example.com") || !strings.Contains(v, "会员") {
		t.Fatalf("登录后应显示会员状态\n%s", v)
	}
}

func TestPointerShape(t *testing.T) {
	var out strings.Builder
	m := start(t, newFake(), Options{Pointer: &out})
	move := func(x, y int) { m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion}) }
	move(5, 2) // 左栏来源
	if out.String() != pointerHand {
		t.Fatalf("悬停来源应切换为小手，got %q", out.String())
	}
	move(6, 2)
	move(30, 3) // 列表条目
	if out.String() != pointerHand {
		t.Fatalf("在可点元素之间移动不应重复发送，got %q", out.String())
	}
	move(5, 15) // 左栏空白
	if out.String() != pointerHand+pointerReset {
		t.Fatalf("移到空白应恢复一次，got %q", out.String())
	}
	move(5, 16)
	move(10, 19) // 底部栏
	if out.String() != pointerHand+pointerReset {
		t.Fatalf("在空白处移动不应重复发送，got %q", out.String())
	}
	out.Reset()
	move(m.menuSpans()[1].x0, 0) // 顶部分类
	move(1, 0)                   // 品牌标题
	if out.String() != pointerHand+pointerReset {
		t.Fatalf("分类可点、品牌标题不可点，got %q", out.String())
	}
	out.Reset()
	move(30, 3)
	move(m.leftWidth(), 3) // 分隔线
	if out.String() != pointerHand+pointerReset {
		t.Fatalf("分类可点、分隔线不可点，got %q", out.String())
	}
}

// threeSources 在综合菜单里加第三个来源，便于测试连续切换与停在末尾。
func threeSources() *fakeSource {
	f := newFake()
	f.data[1].tabs = append(f.data[1].tabs, api.Tab{Key: "douyin", Name: "抖音"})
	f.lists["|douyin|"] = []cli.ListItem{{Title: "抖音第一", URL: "https://example.com/d1"}}
	return f
}

// TestLeftPaneLiveSwitch 左栏 ↑↓ 时右栏实时跟着切换，停下防抖后只请求一次；到首尾停住。
func TestLeftPaneLiveSwitch(t *testing.T) {
	f := threeSources()
	m := start(t, f, Options{})
	n := len(f.listCalls)
	key(t, m, "k")
	if m.srcCur != 0 || m.debPending || len(f.listCalls) != n {
		t.Fatalf("第一个来源上应停住：cur=%d", m.srcCur)
	}
	key(t, m, "j", "down")
	if m.srcCur != 2 || m.focus != focusTree || len(f.listCalls) != n || len(m.items) != 0 || m.target.tab != "douyin" {
		t.Fatalf("应移到第三个来源、清空右栏且尚未请求：cur=%d calls=%v", m.srcCur, f.listCalls[n:])
	}
	if v := plainView(m); !strings.Contains(v, "抖音") || strings.Contains(v, "知乎第一") {
		t.Fatalf("右栏标题应跟着切换\n%s", v)
	}
	settle(t, m)
	if len(f.listCalls) != n+1 || f.listCalls[n] != "|douyin|" || m.items[0].title != "抖音第一" || m.focus != focusTree {
		t.Fatalf("防抖后应只加载最后停下的来源：%v", f.listCalls[n:])
	}
}

// TestRemovedKeys J、K 与 shift+↑↓ 已删除，不再切换来源。
func TestRemovedKeys(t *testing.T) {
	f := threeSources()
	m := start(t, f, Options{})
	key(t, m, "l")
	n := len(f.listCalls)
	key(t, m, "J", "shift+down", "K", "shift+up", "J")
	if m.srcCur != 0 || m.target.tab != "zhihu" || len(f.listCalls) != n || m.focus != focusList || m.listCur != 0 {
		t.Fatalf("已删除的键不应有效果：cur=%d list=%d calls=%v", m.srcCur, m.listCur, f.listCalls[n:])
	}
	m.Update(tea.WindowSizeMsg{Width: 48, Height: 12})
	key(t, m, "J", "tab")
	if m.picking() || m.srcCur != 0 {
		t.Fatal("单栏下 J、tab 不应打开来源列表")
	}
}

// TestHints 底部提示分左栏、榜单两种；放不下时从后往前去掉，? 与 q 始终保留。
func TestHints(t *testing.T) {
	f := newFake()
	m := start(t, f, Options{})
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 20})
	key(t, m, "j")
	settle(t, m)
	st := func() string { return strings.TrimSpace(golden.StripANSI(m.statusLine())) }
	if want := "↑↓ 选来源  → 进入榜单  tab 子榜  0-2 分类  / 搜索  ? 帮助  q 退出"; !strings.HasSuffix(st(), want) {
		t.Fatalf("左栏提示：%q", st())
	}
	key(t, m, "l")
	// 看不到预览时不提示翻预览。
	if want := "↑↓ 移动  ← 来源  tab 子榜  ↵ 打开  0-2 分类  / 搜索  ? 帮助  q 退出"; !strings.HasSuffix(st(), want) {
		t.Fatalf("榜单提示：%q", st())
	}
	// 只有一个子榜单时不显示 tab。
	key(t, m, "h", "k")
	settle(t, m)
	if strings.Contains(st(), "tab") {
		t.Fatalf("没有子榜单时不显示 tab：%q", st())
	}
	// 搜索结果里多一个 esc。
	key(t, m, "/", "x", "enter")
	if !strings.Contains(st(), "↵ 打开  esc 返回榜单") {
		t.Fatalf("搜索结果提示：%q", st())
	}
}

// TestHintsPriority：窄窗口先缩短账号状态，再按重要性去掉提示；← 来源 / → 进入榜单 最后才去掉。
func TestHintsPriority(t *testing.T) {
	f := newFake()
	f.account = &cli.AccountView{ID: 1, Email: "admin@rebang.today", IsMember: true, MemberExpiresAt: "2027-05-13T23:59:00+08:00"}
	m := start(t, f, Options{LoggedIn: true})
	st := func() string { return strings.TrimSpace(golden.StripANSI(m.statusLine())) }
	key(t, m, "j")
	settle(t, m)
	for _, focus := range []string{"tree", "list"} {
		if focus == "list" {
			key(t, m, "l")
		}
		nav := map[string]string{"tree": "→ 进入榜单", "list": "← 来源"}[focus]
		for _, w := range []int{160, 100, 80, 60, 56, 48, 40} {
			m.Update(tea.WindowSizeMsg{Width: w, Height: 20})
			if focus == "tree" && w < 56 && m.focus != focusTree {
				key(t, m, "left") // 单栏打开来源列表
			}
			s := st()
			if !strings.HasSuffix(s, "? 帮助  q 退出") || lipgloss.Width(m.statusLine()) != w {
				t.Fatalf("%s %d 列应保留 ? 与 q：%q", focus, w, s)
			}
			if !strings.Contains(s, nav) {
				t.Fatalf("%s %d 列应保留 %s：%q", focus, w, nav, s)
			}
			if !strings.HasPrefix(s, "●") {
				t.Fatalf("%d 列应保留账号状态：%q", w, s)
			}
			// 低优先级（分类、搜索、打开）先于中优先级（移动、子榜）去掉。
			if (strings.Contains(s, "↑↓") || strings.Contains(s, "tab 子榜")) && !strings.Contains(s, "/ 搜索") && strings.Contains(s, "0-2 分类") {
				t.Fatalf("%d 列去掉顺序不对：%q", w, s)
			}
			if !strings.Contains(s, "0-2 分类") && strings.Contains(s, "/ 搜索") {
				t.Fatalf("同等重要时先去掉靠后的：%q", s)
			}
		}
		// 账号状态逐级缩短：先去掉到期时间。
		m.Update(tea.WindowSizeMsg{Width: 160, Height: 20})
		m.focus = focusTree
		if s := st(); !strings.Contains(s, "admin@rebang.today · 会员，") {
			t.Fatalf("宽屏显示完整账号：%q", s)
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	if s := st(); strings.Contains(s, "到期") || !strings.Contains(s, "· 会员") || !strings.Contains(s, "/ 搜索") {
		t.Fatalf("100 列先去掉到期时间，保留全部提示：%q", s)
	}
	m.Update(tea.WindowSizeMsg{Width: 48, Height: 20})
	if s := st(); !strings.HasPrefix(s, "● 会员  ") {
		t.Fatalf("48 列只留“● 会员”：%q", s)
	}
}

// TestHintsPreview：预览关闭或放得下时不提示翻预览；整栏预览有自己的提示。
func TestHintsPreview(t *testing.T) {
	m := start(t, insightFake(), Options{Now: fixedNow})
	st := func() string { return strings.TrimSpace(golden.StripANSI(m.statusLine())) }
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	key(t, m, "l")
	waitInsight(t, m)
	if !strings.Contains(st(), "↵ 打开  空格 翻预览") {
		t.Fatalf("内容放不下时提示翻预览：%q", st())
	}
	key(t, m, "j") // 第二条只有简介，放得下
	if strings.Contains(st(), "空格") {
		t.Fatalf("放得下时不提示：%q", st())
	}
	key(t, m, "k", "p")
	if strings.Contains(st(), "空格") {
		t.Fatalf("预览关闭时不提示：%q", st())
	}
	key(t, m, "p")
	m.Update(tea.WindowSizeMsg{Width: 48, Height: 16})
	key(t, m, "p")
	if want := "↑↓ 换条  空格 翻页  p/esc 返回  ? 帮助  q 退出"; !strings.HasSuffix(st(), want) {
		t.Fatalf("整栏预览提示：%q", st())
	}
}
