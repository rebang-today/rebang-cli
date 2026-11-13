package tui

import (
	"context"
	"errors"
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
	following []api.Node
	lists     map[string][]cli.ListItem
	listErr   error
	listCalls []string
	search    []cli.SearchResultItem
	searchQ   string
	account   *cli.AccountView
	acctErr   error
	manifest  *update.Manifest
	installed string
}

func (f *fakeSource) Menus(context.Context) ([]api.Menu, error) { return f.menus, nil }
func (f *fakeSource) MenuData(_ context.Context, id int64) (*menuData, error) {
	if f.menuErr != nil {
		return nil, f.menuErr
	}
	return f.data[id], nil
}
func (f *fakeSource) Following(context.Context) ([]api.Node, error) { return f.following, nil }
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
			"|zhihu|":     {{Title: "知乎第一", URL: "https://example.com/z1", Heat: "100万"}, {Title: "知乎第二", URL: "https://example.com/z2"}},
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
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
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
	if m.focus != focusTree || m.srcCur != 1 {
		t.Fatalf("启动后焦点应在左栏的第一个榜单，focus=%v cur=%d", m.focus, m.srcCur)
	}
	v := m.View()
	for _, want := range []string{"1 综合", "2 科技", "★ 我的关注", "知乎第一", "100万", "2 条", "未登录", "/ 搜索"} {
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
	if m.focus != focusTree {
		t.Fatal("列表中 Esc 应回到左栏")
	}
	key(t, m, "tab")
	if m.focus != focusList {
		t.Fatal("tab 应切换到列表")
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
	key(t, m, "l")
	if m.target.tab != "zhihu" || m.subIdx != 0 {
		t.Fatal("没有子榜单时 ←→ 不应有变化")
	}
	key(t, m, "j", "enter", "right")
	if m.target.subTab != "news" || m.items[0].title != "微博要闻" || m.focus != focusList {
		t.Fatalf("右栏中 → 应切换子榜单，target=%+v", m.target)
	}
	key(t, m, "right")
	if m.target.subTab != "news" {
		t.Fatal("最后一个子榜单再按 → 应停住")
	}
	key(t, m, "esc", "h")
	if m.target.subTab != "hot" || m.focus != focusTree {
		t.Fatalf("左栏中 ← 也应切换子榜单，target=%+v", m.target)
	}
	// 子榜单按来源记住。
	key(t, m, "l", "k")
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
	key(t, m, "j", "l") // → 立即加载，取消等待中的延迟
	if m.debPending || m.target.subTab != "news" {
		t.Fatalf("target=%+v", m.target)
	}
	key(t, m, "2")
	if m.menuIdx != 1 || m.target.node != "n1" || m.items[0].title != "少数派文章" {
		t.Fatalf("2 应切换到科技并打开第一个节点，target=%+v", m.target)
	}
	if m.focus != focusTree || m.hasStrip() {
		t.Fatal("切换菜单不改变焦点；节点没有子榜单条")
	}
	key(t, m, "1")
	if m.srcCur != 2 || m.target.tab != "weibo" || m.target.subTab != "news" || m.items[0].title != "微博要闻" {
		t.Fatalf("回到综合应恢复微博 · 要闻，cur=%d target=%+v", m.srcCur, m.target)
	}
	key(t, m, "]")
	if m.menuIdx != 1 {
		t.Fatal("] 应切换到下一个分类")
	}
	key(t, m, "]")
	if m.menuIdx != 0 {
		t.Fatal("] 在最后一个分类应回到第一个")
	}
	key(t, m, "[")
	if m.menuIdx != 1 {
		t.Fatal("[ 应切换到上一个分类")
	}
	key(t, m, "9")
	if m.menuIdx != 1 {
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
		t.Fatalf("空列表应提示换一个榜单\n%s", v)
	}
}

func TestFollowingNotLoggedIn(t *testing.T) {
	f := newFake()
	m := start(t, f, Options{})
	key(t, m, "k")
	if m.srcCur != 0 || m.debPending {
		t.Fatal("选中我的关注不需要加载")
	}
	if v := m.View(); !strings.Contains(v, "登录后可查看关注的榜单") || !strings.Contains(v, "rebang auth login") {
		t.Fatalf("未登录时应提示登录\n%s", v)
	}
}

func TestOpenCopyAndRefresh(t *testing.T) {
	f := newFake()
	var opened, copied string
	m := start(t, f, Options{
		Open: func(u string) error { opened = u; return nil },
		Copy: func(u string) error { copied = u; return nil },
	})
	key(t, m, "tab", "j", "enter")
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

func TestLoggedInShowsFollowingAndMembership(t *testing.T) {
	f := newFake()
	f.following = []api.Node{{NodeID: "n1", Name: "少数派"}, {NodeID: "n2", Name: "虎嗅"}}
	f.lists["n2||"] = []cli.ListItem{{Title: "虎嗅文章", URL: "https://example.com/h1"}}
	f.account = &cli.AccountView{ID: 1, Email: "u@example.com", IsMember: true, MemberExpiresAt: "2027-01-01T00:00:00+08:00"}
	m := start(t, f, Options{LoggedIn: true})
	v := m.View()
	if !strings.Contains(v, "我的关注") || !strings.Contains(v, "u@example.com") || !strings.Contains(v, "会员") {
		t.Fatalf("登录后应显示关注与会员状态\n%s", v)
	}
	key(t, m, "g")
	settle(t, m)
	if m.target.node != "n1" || m.items[0].title != "少数派文章" {
		t.Fatalf("我的关注应打开第一个关注的榜单，target=%+v", m.target)
	}
	if v := m.View(); !strings.Contains(v, "虎嗅") {
		t.Fatalf("关注的榜单应显示在子榜单条\n%s", v)
	}
	key(t, m, "l")
	if m.target.node != "n2" || m.items[0].title != "虎嗅文章" {
		t.Fatalf("→ 应切换到下一个关注，target=%+v", m.target)
	}

	f2 := newFake()
	m = start(t, f2, Options{LoggedIn: true})
	key(t, m, "g")
	if !strings.Contains(m.View(), "还没有关注的榜单") {
		t.Fatalf("没有关注时应提示\n%s", m.View())
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
	click(t, m, 2, 3) // 左栏第 3 项：微博
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
		if sp.idx == 1 {
			menuX = sp.x0 + 1
		}
	}
	click(t, m, menuX, 0)
	if m.menuIdx != 1 || m.target.node != "n1" || m.focus != focusTree {
		t.Fatalf("点击菜单应切换分类，menu=%d target=%+v", m.menuIdx, m.target)
	}
	_, _ = m.Update(tea.MouseMsg{X: 2, Y: 3, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
}

func TestNarrowLayout(t *testing.T) {
	f := newFake()
	m := start(t, f, Options{})
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 12})
	v := m.View()
	if strings.Contains(v, "│") || !strings.Contains(v, "★ 我的关注") || strings.Contains(v, "知乎第一") {
		t.Fatalf("窄窗口启动时应只显示左栏\n%s", v)
	}
	key(t, m, "tab")
	if v := m.View(); !strings.Contains(v, "知乎第一") || strings.Contains(v, "★ 我的关注") {
		t.Fatalf("tab 应切换到列表\n%s", v)
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
		key(t, m, "l")
	}
	key(t, m, "tab")
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
		if sp.idx == 2 {
			click(t, m, sp.x0, 0)
		}
	}
	if m.menuIdx != 2 {
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
