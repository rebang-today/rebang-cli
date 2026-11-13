package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/cli"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/update"
)

type fakeSource struct {
	menus     []api.Menu
	data      map[int64]*menuData
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
	v := m.View()
	for _, want := range []string{"综合", "知乎", "知乎第一", "100万", "未登录", "/ 搜索"} {
		if !strings.Contains(v, want) {
			t.Errorf("界面缺少 %q\n%s", want, v)
		}
	}
	if got := len(strings.Split(v, "\n")); got != 20 {
		t.Errorf("界面行数 = %d, 想要 20", got)
	}
}

func TestTreeNavigationAndSubTabs(t *testing.T) {
	f := newFake()
	m := start(t, f, Options{})
	key(t, m, "tab") // 回到左栏
	if m.focus != focusTree {
		t.Fatal("tab 应切换到左栏")
	}
	key(t, m, "j", "enter") // 微博（有二级 Tab）
	if m.target.tab != "weibo" || m.target.subTab != "hot" {
		t.Fatalf("有二级 Tab 时应打开第一个，target=%+v", m.target)
	}
	if m.focus != focusList || m.items[0].title != "微博热搜" {
		t.Fatalf("应聚焦右栏并显示榜单，items=%+v", m.items)
	}
	key(t, m, "h", "j", "l")
	if m.target.subTab != "news" {
		t.Fatalf("l 应打开二级 Tab，target=%+v", m.target)
	}
	// 收起再展开第二个菜单，加载节点。
	key(t, m, "tab", "G", "enter")
	e, _ := m.treeEntry()
	if e.kind != kindMenu || e.menuID != 2 || !m.tree.expanded[menuKey(2)] {
		t.Fatalf("回车应展开菜单，entry=%+v", e)
	}
	key(t, m, "j", "enter")
	if m.target.node != "n1" || m.items[0].title != "少数派文章" {
		t.Fatalf("应打开节点榜单，target=%+v", m.target)
	}
}

func TestOpenCopyAndRefresh(t *testing.T) {
	f := newFake()
	var opened, copied string
	m := start(t, f, Options{
		Open: func(u string) error { opened = u; return nil },
		Copy: func(u string) error { copied = u; return nil },
	})
	key(t, m, "j", "enter")
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
	f.following = []api.Node{{NodeID: "n1", Name: "少数派"}}
	f.account = &cli.AccountView{ID: 1, Email: "u@example.com", IsMember: true, MemberExpiresAt: "2027-01-01T00:00:00+08:00"}
	m := start(t, f, Options{LoggedIn: true})
	v := m.View()
	if !strings.Contains(v, "我的关注") || !strings.Contains(v, "u@example.com") || !strings.Contains(v, "会员") {
		t.Fatalf("登录后应显示关注与会员状态\n%s", v)
	}
	key(t, m, "tab", "g", "enter")
	if !m.tree.followOpen || m.rows[1].nodeID != "n1" {
		t.Fatalf("应展开我的关注，rows=%+v", m.rows[:2])
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

func TestMouse(t *testing.T) {
	f := newFake()
	var opened string
	now := time.Unix(1700000000, 0)
	m := start(t, f, Options{Open: func(u string) error { opened = u; return nil }, Now: func() time.Time { return now }})
	lw := m.leftWidth()
	click := func(x, y int) {
		_, cmd := m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		drain(t, m, cmd)
	}
	click(lw+5, 2) // 第二条
	if m.listCur != 1 || opened != "" {
		t.Fatalf("单击应选中，cur=%d opened=%q", m.listCur, opened)
	}
	now = now.Add(200 * time.Millisecond)
	click(lw+5, 2)
	if opened != "https://example.com/z2" {
		t.Fatalf("双击应打开，opened=%q", opened)
	}
	click(2, 3) // 左栏第 3 行：微博
	if m.target.tab != "weibo" {
		t.Fatalf("点击左栏应打开榜单，target=%+v", m.target)
	}
	_, _ = m.Update(tea.MouseMsg{X: 2, Y: 3, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
}

func TestNarrowLayout(t *testing.T) {
	f := newFake()
	m := start(t, f, Options{})
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 12})
	v := m.View()
	if strings.Contains(v, "│") || !strings.Contains(v, "知乎第一") || strings.Contains(v, "▾ 综合") {
		t.Fatalf("窄窗口应只显示右栏\n%s", v)
	}
	key(t, m, "h")
	if v := m.View(); !strings.Contains(v, "▾ 综合") {
		t.Fatalf("h 应切换到左栏\n%s", v)
	}
	for _, line := range strings.Split(v, "\n") {
		if w := len([]rune(line)); w > 400 {
			t.Fatal("行过长")
		}
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
