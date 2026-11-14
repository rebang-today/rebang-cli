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

// waitInsight 模拟光标停下约 200ms 后的定时（drain 不等待超过 100ms 的定时）。
func waitInsight(t *testing.T, m *model) {
	t.Helper()
	if m.pvWait == "" {
		t.Fatal("应有等待中的完整总结请求")
	}
	_, cmd := m.Update(insightWaitMsg{m.insSeq, m.pvWait})
	drain(t, m, cmd)
}

// previewText 返回当前条目预览的全部纯文本行。
func previewText(m *model) string {
	w, _ := m.previewBox()
	return golden.StripANSI(strings.Join(m.previewContent(w), "\n"))
}

func TestInsightShorts(t *testing.T) {
	f := insightFake()
	m := start(t, f, Options{LoggedIn: true, Now: fixedNow})
	if len(f.shortsCalls) != 1 || strings.Join(f.shortsCalls[0], ",") != "kz1,kz2,kz3" {
		t.Fatalf("列表加载后应批量查询本页 item_key：%v", f.shortsCalls)
	}
	if v := plainView(m); strings.Contains(v, "✦") || !strings.Contains(lineWith(v, "知乎第一"), "1  知乎第一") {
		t.Fatalf("列表不再显示 ✦ 标记\n%s", v)
	}
	// 刷新不重复查询已经查过的 item_key。
	key(t, m, "r")
	if len(f.shortsCalls) != 1 {
		t.Fatalf("已查询过的条目不应重复请求：%v", f.shortsCalls)
	}

	// 未登录：接口不要求登录，照常查询。
	f2 := insightFake()
	start(t, f2, Options{Now: fixedNow})
	if len(f2.shortsCalls) != 1 {
		t.Fatalf("未登录也应查询：%v", f2.shortsCalls)
	}

	// 登录失效后不再带着失效令牌请求。
	f4 := insightFake()
	f4.shortsErr = errs.Unauthorized("")
	m4 := start(t, f4, Options{LoggedIn: true, Now: fixedNow})
	f4.shortsErr = nil
	key(t, m4, "l", "j")
	if !m4.expired || len(f4.shortsCalls) != 1 {
		t.Fatalf("401 后不应再请求：%v %v", m4.expired, f4.shortsCalls)
	}

	// 批量查询失败静默，不提示错误。
	f3 := insightFake()
	f3.shortsErr = errs.Network(errors.New("refused"))
	m3 := start(t, f3, Options{LoggedIn: true, Now: fixedNow})
	if m3.isError {
		t.Fatalf("批量失败应静默：%q", m3.message)
	}
}

func TestPreviewContent(t *testing.T) {
	f := insightFake()
	m := start(t, f, Options{LoggedIn: true, Now: fixedNow})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	key(t, m, "l")
	v := plainView(m)
	for _, s := range []string{"知乎 · 08:05 · 100万", "AI 总结", "一句话总结第一条", "加载中…", "───"} {
		if !strings.Contains(v, s) {
			t.Fatalf("预览缺少 %q\n%s", s, v)
		}
	}
	if len(f.insightCalls) != 0 {
		t.Fatal("停下 200ms 之前不应请求完整总结")
	}
	waitInsight(t, m)
	if len(f.insightCalls) != 1 || f.insightCalls[0] != "kz1" {
		t.Fatalf("应请求完整总结：%v", f.insightCalls)
	}
	order := []string{"知乎 · 08:05 · 100万", "AI 总结", "一句话总结第一条",
		"要点", "• 第一条关键事实", "经过", "• 9月1日 正式公布", "观点", "• 支持", "成本明显下降", "“我算过账”", "—— 知乎用户A",
		"网友怎么看", "多数人认可", "• 网友甲：早该如此", "“终于等到了”", "简介", "第一条的描述"}
	pt, pos := previewText(m), 0
	for _, s := range order {
		i := strings.Index(pt[pos:], s)
		if i < 0 {
			t.Fatalf("缺少或顺序错误 %q\n%s", s, pt)
		}
		pos += i + len(s)
	}
	if strings.Contains(pt, "金句") || strings.Contains(pt, "**") || strings.Contains(pt, "加载中") {
		t.Fatalf("应忽略 quotes、渲染加粗、去掉加载中\n%s", pt)
	}
	// 没有总结：只有“简介”，不请求完整总结。
	key(t, m, "j")
	if pt := previewText(m); !strings.Contains(pt, "简介\n第二条的描述") || strings.Contains(pt, "AI 总结") || m.pvWait != "" {
		t.Fatalf("没有总结时应显示简介\n%s", pt)
	}
	key(t, m, "j")
	if pt := previewText(m); !strings.Contains(pt, "暂无摘要") || strings.Contains(pt, "简介") {
		t.Fatalf("都没有时应显示暂无摘要\n%s", pt)
	}
	// 回到第一条：使用缓存，不再请求。
	key(t, m, "g")
	if m.pvWait != "" || len(f.insightCalls) != 1 || !strings.Contains(previewText(m), "要点") {
		t.Fatalf("应使用缓存：%v", f.insightCalls)
	}
	// 预览占右栏约 40%，列表仍能滚动到最后一条。
	if ph := m.previewHeight(); ph != max(previewMinRows, m.bodyHeight()*2/5) || m.listHeight() != m.bodyHeight()-m.rightHead()-ph {
		t.Fatal(ph, m.listHeight())
	}
}

func TestPreviewBold(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(old)
	m := start(t, insightFake(), Options{LoggedIn: true, Now: fixedNow})
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
	key(t, m, "l")
	waitInsight(t, m)
	v := m.View()
	if !strings.Contains(v, styleBold.Render("关键")) || !strings.Contains(v, styleQuote.Render("“我算过账”")) {
		t.Fatalf("加粗与引用样式缺失\n%q", v)
	}
	// 小标题样式统一。
	for _, h := range []string{"AI 总结", "简介", "要点"} {
		if !strings.Contains(v, styleInsightHead.Render(h)) {
			t.Fatalf("小标题 %s 样式不一致", h)
		}
	}
}

func TestInsightDebounce(t *testing.T) {
	f := insightFake()
	f.shorts["kz2"] = "第二条总结"
	f.insights["kz2"] = &api.Insight{SummaryShort: "第二条总结"}
	m := start(t, f, Options{LoggedIn: true, Now: fixedNow})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	key(t, m, "l")
	stale := insightWaitMsg{m.insSeq, m.pvWait}
	// 快速移动：只有最后停下的条目会请求。
	key(t, m, "j", "k", "j")
	_, cmd := m.Update(stale)
	drain(t, m, cmd)
	if len(f.insightCalls) != 0 {
		t.Fatalf("过期的定时不应请求：%v", f.insightCalls)
	}
	waitInsight(t, m)
	if strings.Join(f.insightCalls, ",") != "kz2" {
		t.Fatalf("只应请求停下的条目：%v", f.insightCalls)
	}
	key(t, m, "k", "j")
	if m.pvWait != "" {
		t.Fatal("已缓存的条目不应再等待")
	}
	key(t, m, "k")
	waitInsight(t, m)
	_, cmd = m.Update(insightWaitMsg{m.insSeq, "kz1"})
	drain(t, m, cmd)
	if strings.Join(f.insightCalls, ",") != "kz2,kz1" {
		t.Fatalf("每条只请求一次：%v", f.insightCalls)
	}
	// 预览关闭时不请求。
	f2 := insightFake()
	m2 := start(t, f2, Options{LoggedIn: true, Now: fixedNow})
	if m2.pvWait != "" || m2.previewShown() {
		t.Fatal("看不到预览时不应请求完整总结")
	}
}

func TestInsightFailureFallsBack(t *testing.T) {
	f := insightFake()
	f.insightErr = errs.Network(errors.New("refused"))
	m := start(t, f, Options{LoggedIn: true, Now: fixedNow})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	key(t, m, "l")
	waitInsight(t, m)
	pt := previewText(m)
	if !strings.Contains(pt, "一句话总结第一条") || strings.Contains(pt, "加载中") || strings.Contains(pt, "✗") || m.isError {
		t.Fatalf("失败时只显示一句话总结\n%s", pt)
	}
	key(t, m, "j", "k")
	if m.pvWait != "" || len(f.insightCalls) != 1 {
		t.Fatal("失败结果也缓存，不反复请求")
	}

	// 404（没有完整总结）：同样只显示一句话总结。
	f2 := insightFake()
	delete(f2.insights, "kz1")
	m2 := start(t, f2, Options{LoggedIn: true, Now: fixedNow})
	m2.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	key(t, m2, "l")
	waitInsight(t, m2)
	if pt := previewText(m2); !strings.Contains(pt, "一句话总结第一条") || strings.Contains(pt, "加载中") {
		t.Fatalf("404 时只显示一句话总结\n%s", pt)
	}

	// 401：标记登录失效，不显示加载中。
	f4 := insightFake()
	f4.insightErr = errs.Unauthorized("")
	m4 := start(t, f4, Options{LoggedIn: true, Now: fixedNow})
	m4.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	key(t, m4, "l")
	waitInsight(t, m4)
	if pt := previewText(m4); !m4.expired || strings.Contains(pt, "加载中") || !strings.Contains(pt, "一句话总结第一条") {
		t.Fatalf("401 应标记登录失效\n%s", pt)
	}

	// 未登录：接口不要求登录，照常请求。
	f3 := insightFake()
	m3 := start(t, f3, Options{Now: fixedNow})
	m3.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	key(t, m3, "l")
	waitInsight(t, m3)
	if len(f3.insightCalls) != 1 || !strings.Contains(previewText(m3), "要点") {
		t.Fatalf("未登录也应显示完整总结：%v", f3.insightCalls)
	}
}

func TestPreviewPaging(t *testing.T) {
	f := insightFake()
	m := start(t, f, Options{LoggedIn: true, Now: fixedNow})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	key(t, m, "l")
	waitInsight(t, m)
	w, h := m.previewBox()
	n := len(m.previewContent(w))
	if n <= h || !strings.Contains(plainView(m), "↓ 更多") {
		t.Fatalf("内容应超出预览区并提示更多：%d/%d\n%s", n, h, plainView(m))
	}
	pages := 0
	for {
		key(t, m, " ")
		pages++
		if m.pvOff == 0 || pages > 20 {
			break
		}
		if m.pvOff+h < n && !strings.Contains(plainView(m), "↓ 更多") {
			t.Fatal("未到底时应提示更多")
		}
		if m.pvOff+h >= n && strings.Contains(plainView(m), "↓ 更多") {
			t.Fatal("到底后不再提示更多")
		}
	}
	if want := (n-h+h-1)/h + 1; pages != want || m.listCur != 0 {
		t.Fatalf("空格应逐屏翻到底再回到开头：%d 次，期望 %d", pages, want)
	}
	if !strings.Contains(plainView(m), "知乎 · 08:05 · 100万") {
		t.Fatal("回到开头")
	}
	// 换条目后回到顶部；放得下时空格不做任何事。
	key(t, m, " ")
	key(t, m, "j")
	if m.pvOff != 0 {
		t.Fatal("换条目后应回到顶部")
	}
	key(t, m, " ")
	if m.pvOff != 0 || m.listCur != 1 {
		t.Fatal("内容放得下时空格不做任何事")
	}
}

func TestPreviewLayouts(t *testing.T) {
	m := start(t, insightFake(), Options{LoggedIn: true, Now: fixedNow})
	key(t, m, "l")
	// 129 列：预览在榜单下方。
	m.Update(tea.WindowSizeMsg{Width: 129, Height: 30})
	if m.previewSide() || m.previewHeight() == 0 || strings.Count(lineWith(plainView(m), "知乎第一"), "│") != 1 {
		t.Fatalf("129 列应为下方预览\n%s", plainView(m))
	}
	// 130 列：三栏，预览约占右侧 40%，至少 36 列。
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 30})
	sw := m.sideWidth()
	if !m.previewSide() || m.previewHeight() != 0 || sw < sidePaneMin || sw != max(sidePaneMin, (130-m.leftWidth()-1)*2/5) {
		t.Fatalf("130 列应为三栏：side=%d", sw)
	}
	if m.leftWidth()+1+m.rightWidth()+1+sw != 130 {
		t.Fatal("三栏宽度应正好铺满")
	}
	v := plainView(m)
	if l := lineWith(v, "知乎第一"); strings.Count(l, "│") != 2 {
		t.Fatalf("三栏应有两条分隔线\n%s", v)
	}
	if !strings.Contains(v, "一句话总结第一条") || strings.Contains(v, "───") {
		t.Fatalf("三栏预览\n%s", v)
	}
	// 预览栏不参与点击与切栏。
	if h := m.hitTest(129, 3); h.what != hitNone {
		t.Fatalf("点击预览栏不应选中条目：%+v", h)
	}
	key(t, m, "right")
	if m.focus != focusList {
		t.Fatal("→ 不进入预览栏")
	}
	// 三栏也遵守最小高度；不足 16 行时退回下方预览的规则（不足 24 行不显示）。
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 15})
	if m.previewSide() || m.previewHeight() != 0 {
		t.Fatal("130x15 不显示预览")
	}
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 16})
	if !m.previewSide() {
		t.Fatal("130x16 应为三栏")
	}
	// p 关闭三栏预览，榜单变宽。
	key(t, m, "p")
	if m.previewSide() || m.rightWidth() != 130-m.leftWidth()-1 {
		t.Fatal("p 应关闭三栏预览")
	}
	key(t, m, "p")
	if !m.previewSide() {
		t.Fatal("p 应重新打开")
	}
}

func TestPreviewHiddenWhenShort(t *testing.T) {
	m := start(t, insightFake(), Options{LoggedIn: true, Now: fixedNow})
	for _, sz := range [][2]int{{48, 30}, {100, 23}} {
		m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		if m.previewShown() || strings.Contains(plainView(m), "───") || strings.Contains(plainView(m), "一句话总结") {
			t.Fatalf("%v 不应显示预览\n%s", sz, plainView(m))
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	if m.previewHeight() != previewMinRows+2 {
		t.Fatalf("24 行时预览占 40%%：%d", m.previewHeight())
	}
	// 预览区域不响应列表点击。
	y := 1 + m.rightHead() + m.listHeight() + 2
	if h := m.hitTest(m.rightX()+5, y); h.what == hitItem {
		t.Fatalf("点击预览不应选中条目：%+v", h)
	}
}

func TestPreviewNarrow(t *testing.T) {
	f := insightFake()
	m := start(t, f, Options{LoggedIn: true, Now: fixedNow})
	m.Update(tea.WindowSizeMsg{Width: 48, Height: 20})
	if m.previewShown() {
		t.Fatal("单栏默认不显示预览")
	}
	key(t, m, "p")
	v := plainView(m)
	if !m.previewFull() || !strings.Contains(v, "知乎第一") || !strings.Contains(v, "AI 总结") || strings.Contains(v, "知乎第二") {
		t.Fatalf("p 应打开整栏预览\n%s", v)
	}
	waitInsight(t, m)
	// ↑↓ 仍移动榜单光标，预览跟着刷新。
	key(t, m, "j")
	if v := plainView(m); m.listCur != 1 || !strings.Contains(v, "第二条的描述") || !m.previewFull() {
		t.Fatalf("↓ 应移动光标并刷新预览\n%s", v)
	}
	key(t, m, "k", " ")
	if m.pvOff == 0 {
		t.Fatal("空格应翻页")
	}
	key(t, m, "esc")
	if m.previewFull() || !strings.Contains(plainView(m), "知乎第二") {
		t.Fatal("Esc 应回到榜单")
	}
	key(t, m, "p", "p")
	if m.previewFull() {
		t.Fatal("p 应回到榜单")
	}
	if m.previewOff {
		t.Fatal("单栏 p 不改变宽屏的预览开关")
	}
	// ← 关闭整栏预览并打开来源列表；变宽后不再是整栏预览。
	key(t, m, "p", "left")
	if m.previewFull() || !m.picking() {
		t.Fatal("← 应打开来源列表")
	}
	key(t, m, "right", "p")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(tea.WindowSizeMsg{Width: 48, Height: 20})
	if m.previewFull() {
		t.Fatal("窗口变化后回到榜单")
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
	m2.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	if !m2.previewOff || m2.previewSide() {
		t.Fatal("重新启动后应保持关闭（三栏同样）")
	}
	key(t, m2, "p")
	m2.SaveState()
	if m3 := start(t, insightFake(), Options{StatePath: path}); m3.previewOff {
		t.Fatal("再次打开后应保存为打开")
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
