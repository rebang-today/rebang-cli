package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/cli"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/golden"
)

const insightDetail = `{
  "quotes": ["旧版金句不显示"],
  "reactions": {"mood": "多数人认可", "voices": [{"by": "网友甲", "text": "早该如此", "quote": "终于等到了"}]},
  "viewpoints": [{"stance": "支持", "text": "成本**明显下降**", "by": "知乎用户A", "quote": "我算过账"}],
  "timeline": [{"time": "9月1日", "text": "正式公布"}],
  "points": ["第一条**关键**事实", "第二条事实"]
}`

func insightFake() *fakeSource {
	f := newFake()
	f.lists["|zhihu|"] = []cli.ListItem{
		{Title: "知乎第一", URL: "https://example.com/z1", Heat: "100万", ItemKey: "kz1", Desc: "第一条的描述", PublishedAt: "2026-11-13T08:05:00Z"},
		{Title: "知乎第二", URL: "https://example.com/z2", ItemKey: "kz2", Desc: "第二条的描述"},
		{Title: "知乎第三", URL: "https://example.com/z3", ItemKey: "kz3"},
	}
	f.shorts = map[string]string{"kz1": "一句话总结第一条"}
	f.insights = map[string]*api.Insight{"kz1": {SummaryShort: "一句话总结第一条", Detail: json.RawMessage(insightDetail)}}
	return f
}

func plainView(m *model) string { return golden.StripANSI(m.View()) }

func lineWith(v, s string) string {
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	return ""
}

func TestInsightMarkers(t *testing.T) {
	f := insightFake()
	m := start(t, f, Options{LoggedIn: true, Now: fixedNow})
	if len(f.shortsCalls) != 1 || strings.Join(f.shortsCalls[0], ",") != "kz1,kz2,kz3" {
		t.Fatalf("列表加载后应批量查询本页 item_key：%v", f.shortsCalls)
	}
	v := plainView(m)
	if !strings.Contains(lineWith(v, "知乎第一"), "1 "+insightMark+"知乎第一") || strings.Contains(lineWith(v, "知乎第二"), insightMark) {
		t.Fatalf("有总结的条目标题前应有标记\n%s", v)
	}
	// 刷新不重复查询已经查过的 item_key。
	key(t, m, "r")
	if len(f.shortsCalls) != 1 {
		t.Fatalf("已查询过的条目不应重复请求：%v", f.shortsCalls)
	}

	// 未登录：不发批量请求，不显示标记。
	f2 := insightFake()
	m2 := start(t, f2, Options{Now: fixedNow})
	if len(f2.shortsCalls) != 0 || strings.Contains(plainView(m2), insightMark) {
		t.Fatalf("未登录不应查询：%v", f2.shortsCalls)
	}

	// 批量查询失败静默，不提示错误。
	f3 := insightFake()
	f3.shortsErr = errs.Network(errors.New("refused"))
	m3 := start(t, f3, Options{LoggedIn: true, Now: fixedNow})
	if m3.isError || strings.Contains(plainView(m3), insightMark) {
		t.Fatalf("批量失败应静默：%q", m3.message)
	}
}

func TestPreviewContent(t *testing.T) {
	f := insightFake()
	m := start(t, f, Options{LoggedIn: true, Now: fixedNow})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	key(t, m, "tab")
	v := plainView(m)
	for _, s := range []string{"知乎 · 08:05 · 100万", "AI 总结 一句话总结第一条", "───"} {
		if !strings.Contains(v, s) {
			t.Fatalf("预览缺少 %q\n%s", s, v)
		}
	}
	key(t, m, "j")
	if v := plainView(m); !strings.Contains(v, "第二条的描述") || strings.Contains(v, "AI 总结") {
		t.Fatalf("没有总结时应显示描述\n%s", v)
	}
	key(t, m, "j")
	if v := plainView(m); !strings.Contains(v, "暂无摘要") {
		t.Fatalf("没有总结和描述时应显示暂无摘要\n%s", v)
	}
	if len(f.shortsCalls) != 1 || len(f.insightCalls) != 0 {
		t.Fatalf("移动光标不应发请求：%v %v", f.shortsCalls, f.insightCalls)
	}
	// 预览占用列表的行数，列表仍能滚动到最后一条。
	if m.listHeight() != m.bodyHeight()-m.rightHead()-previewRows-1 {
		t.Fatal(m.listHeight())
	}
}

func TestPreviewHiddenWhenNarrowOrShort(t *testing.T) {
	m := start(t, insightFake(), Options{LoggedIn: true, Now: fixedNow})
	for _, sz := range [][2]int{{48, 30}, {100, 23}} {
		m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		if m.previewHeight() != 0 || strings.Contains(plainView(m), "───") {
			t.Fatalf("%v 不应显示预览\n%s", sz, plainView(m))
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	if m.previewHeight() == 0 {
		t.Fatal("双栏且 24 行时应显示预览")
	}
	// 预览区域不响应列表点击。
	y := 1 + m.rightHead() + m.listHeight() + 2
	if h := m.hitTest(m.rightX()+5, y); h.what == hitItem {
		t.Fatalf("点击预览不应选中条目：%+v", h)
	}
}

func TestPreviewToggleSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), stateFileName)
	m := start(t, insightFake(), Options{LoggedIn: true, Now: fixedNow, StatePath: path})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	key(t, m, "p")
	if !m.previewOff || m.previewHeight() != 0 || strings.Contains(plainView(m), "───") {
		t.Fatal("p 应关闭预览")
	}
	m.SaveState()
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), `"preview": false`) {
		t.Fatalf("应保存预览开关：%s %v", b, err)
	}
	m2 := start(t, insightFake(), Options{LoggedIn: true, Now: fixedNow, StatePath: path})
	m2.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !m2.previewOff {
		t.Fatal("重新启动后应保持关闭")
	}
	key(t, m2, "p")
	m2.SaveState()
	if m3 := start(t, insightFake(), Options{StatePath: path}); m3.previewOff {
		t.Fatal("再次打开后应保存为打开")
	}
}

func TestDetailSections(t *testing.T) {
	f := insightFake()
	m := start(t, f, Options{LoggedIn: true, Now: fixedNow})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 60})
	key(t, m, " ")
	if m.det == nil || len(f.insightCalls) != 1 || f.insightCalls[0] != "kz1" {
		t.Fatalf("空格应打开详情并加载：%v", f.insightCalls)
	}
	v := plainView(m)
	order := []string{"知乎第一", "知乎 · 08:05 · 100万", "第一条的描述", "AI 总结", "一句话总结第一条",
		"要点", "• 第一条关键事实", "经过", "• 9月1日 正式公布", "观点", "• 支持", "成本明显下降", "“我算过账”", "—— 知乎用户A",
		"网友怎么看", "多数人认可", "• 网友甲：早该如此", "“终于等到了”", "esc 返回"}
	pos := 0
	for _, s := range order {
		i := strings.Index(v[pos:], s)
		if i < 0 {
			t.Fatalf("缺少或顺序错误 %q\n%s", s, v)
		}
		pos += i + len(s)
	}
	if strings.Contains(v, "金句") || strings.Contains(v, "**") {
		t.Fatalf("应忽略 quotes 并渲染加粗\n%s", v)
	}

	// 关闭后再打开同一条，使用缓存。
	key(t, m, "esc")
	if m.det != nil {
		t.Fatal("esc 应关闭详情")
	}
	key(t, m, " ")
	key(t, m, " ")
	if m.det != nil || len(f.insightCalls) != 1 {
		t.Fatalf("空格应关闭详情，且不重复请求：%v", f.insightCalls)
	}
	// 详情加载后列表标记保持。
	if !strings.Contains(plainView(m), insightMark) {
		t.Fatal("标记应保留")
	}
}

func TestDetailBold(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(old)
	m := start(t, insightFake(), Options{LoggedIn: true, Now: fixedNow})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 60})
	key(t, m, " ")
	if v := m.View(); !strings.Contains(v, styleBold.Render("关键")) || !strings.Contains(v, styleQuote.Render("“我算过账”")) {
		t.Fatalf("加粗与引用样式缺失\n%q", v)
	}
}

func TestDetailStates(t *testing.T) {
	// 加载中：命令尚未返回。
	f := insightFake()
	m := start(t, f, Options{LoggedIn: true, Now: fixedNow})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	if v := plainView(m); !strings.Contains(v, "加载中…") {
		t.Fatalf("应显示加载中\n%s", v)
	}
	drain(t, m, cmd)
	if v := plainView(m); strings.Contains(v, "加载中") || !strings.Contains(v, "一句话总结第一条") {
		t.Fatalf("加载完成后应显示总结\n%s", v)
	}

	// 404：暂无 AI 总结。
	key(t, m, "esc", "tab", "j", " ")
	if v := plainView(m); !strings.Contains(v, "暂无 AI 总结") || !strings.Contains(v, "第二条的描述") {
		t.Fatalf("没有总结时应提示\n%s", v)
	}

	// 未登录：提示登录，不发请求。
	f2 := insightFake()
	m2 := start(t, f2, Options{Now: fixedNow})
	key(t, m2, " ")
	if v := plainView(m2); !strings.Contains(v, "登录后可查看 AI 总结（rebang auth login）") || len(f2.insightCalls) != 0 {
		t.Fatalf("未登录应提示登录\n%s", v)
	}

	// 其他错误：一行错误信息。
	f3 := insightFake()
	f3.insightErr = errs.Network(errors.New("refused"))
	m3 := start(t, f3, Options{LoggedIn: true, Now: fixedNow})
	key(t, m3, " ")
	if v := plainView(m3); !strings.Contains(v, "✗ 网络请求失败") {
		t.Fatalf("应显示错误\n%s", v)
	}
	// 出错后重新打开会重试。
	f3.insightErr = nil
	key(t, m3, "esc", " ")
	if len(f3.insightCalls) != 2 || !strings.Contains(plainView(m3), "一句话总结第一条") {
		t.Fatalf("出错后应重试：%v", f3.insightCalls)
	}

	// 401：标记登录失效并提示登录。
	f4 := insightFake()
	f4.insightErr = errs.Unauthorized("")
	m4 := start(t, f4, Options{LoggedIn: true, Now: fixedNow})
	key(t, m4, " ")
	if v := plainView(m4); !m4.expired || !strings.Contains(v, "登录后可查看 AI 总结") {
		t.Fatalf("401 应提示重新登录\n%s", v)
	}
}

func TestDetailScrollAndKeys(t *testing.T) {
	var opened []string
	f := insightFake()
	m := start(t, f, Options{LoggedIn: true, Now: fixedNow, Open: func(u string) error { opened = append(opened, u); return nil }})
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 12})
	key(t, m, " ")
	total := len(m.detailContent(m.detailWidth()))
	if total <= m.bodyHeight() {
		t.Fatalf("内容应超过一屏：%d", total)
	}
	if v := plainView(m); !strings.Contains(v, "↓ ") || !strings.Contains(v, "知乎第一") {
		t.Fatalf("应提示可以滚动\n%s", v)
	}
	key(t, m, "j", "j")
	if m.det.off != 2 {
		t.Fatal(m.det.off)
	}
	key(t, m, "k")
	if m.det.off != 1 {
		t.Fatal(m.det.off)
	}
	key(t, m, "G")
	if m.det.off != total-m.bodyHeight() || !strings.Contains(plainView(m), "终于等到了") {
		t.Fatalf("G 应到底：%d\n%s", m.det.off, plainView(m))
	}
	key(t, m, "g")
	if m.det.off != 0 {
		t.Fatal(m.det.off)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.det.off != m.bodyHeight()-1 {
		t.Fatalf("PgDn 翻页：%d", m.det.off)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m.Update(tea.MouseMsg{X: 5, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if m.det.off != 3 {
		t.Fatalf("滚轮滚动：%d", m.det.off)
	}
	key(t, m, "enter")
	if len(opened) != 1 || opened[0] != "https://example.com/z1" || m.det == nil {
		t.Fatalf("回车应打开原文：%v", opened)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd != nil || m.det != nil {
		t.Fatal("详情页中 q 应关闭详情而不是退出")
	}
	// 详情页的提示随窗口缩小截断，不超宽。
	key(t, m, " ")
	for _, w := range []int{40, 41, 60} {
		m.Update(tea.WindowSizeMsg{Width: w, Height: 10})
		for _, l := range strings.Split(m.View(), "\n") {
			if lipgloss.Width(l) > w {
				t.Fatalf("%d：行宽 %d 超出\n%q", w, lipgloss.Width(l), l)
			}
		}
	}
}

func TestShortTime(t *testing.T) {
	now := fixedNow()
	cases := map[string]string{
		"":                     "",
		"bad":                  "",
		"2026-11-13T08:05:00Z": "08:05",
		"2026-10-01T08:05:00Z": "10月1日 08:05",
		"2025-10-01T08:05:00Z": "2025年10月1日",
	}
	for in, want := range cases {
		if got := shortTime(in, now); got != want {
			t.Errorf("%q → %q want %q", in, got, want)
		}
	}
}

func TestWideHintsShowDetail(t *testing.T) {
	f := newFake()
	f.account = &cli.AccountView{ID: 1, Email: "a-very-long-address@example.com", IsMember: true, MemberExpiresAt: "2027-01-01T00:00:00+08:00"}
	m := newModel(Options{Source: f, Version: "0.1.0", LoggedIn: true})
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	drain(t, m, m.Init())
	check := func(where string) {
		t.Helper()
		s := golden.StripANSI(m.statusLine())
		i, j := strings.Index(s, "空格 详情"), strings.Index(s, "/ 搜索")
		if i < 0 || (j >= 0 && j < i) {
			t.Fatalf("%s：宽屏底部应显示「空格 详情」且排在「/ 搜索」之前\n%s", where, s)
		}
	}
	if m.focus != focusTree {
		t.Fatalf("启动焦点应在左栏：%v", m.focus)
	}
	check("左栏")
	key(t, m, "tab")
	check("列表")
}
