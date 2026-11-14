package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/cli"
)

func wheel(m *model, x, y int, b tea.MouseButton) {
	m.Update(tea.MouseMsg{X: x, Y: y, Button: b, Action: tea.MouseActionPress})
}

// wheelFake 有很多菜单、很多子榜单和很长的榜单，用来测试滚轮。
func wheelFake() *fakeSource {
	f := insightFake()
	for i := 3; i <= 12; i++ {
		f.menus = append(f.menus, api.Menu{ID: int64(i), Name: fmt.Sprintf("分类%02d", i)})
	}
	var subs []api.SubTab
	for i := 0; i < 12; i++ {
		k := fmt.Sprintf("s%02d", i)
		subs = append(subs, api.SubTab{Key: k, Name: fmt.Sprintf("子榜单%02d", i)})
		f.lists["|weibo|"+k] = []cli.ListItem{{Title: "微博" + k, URL: "https://example.com/" + k}}
	}
	f.data[1].tabs[1].Children = subs
	var items []cli.ListItem
	for i := 0; i < 40; i++ {
		items = append(items, cli.ListItem{Title: fmt.Sprintf("条目%02d", i), URL: fmt.Sprintf("https://example.com/%d", i), ItemKey: fmt.Sprintf("k%02d", i)})
	}
	items[0].ItemKey, items[0].Desc = "kz1", "第一条的描述"
	f.lists["|zhihu|"] = items
	return f
}

func TestWheelListKeepsSelection(t *testing.T) {
	m := start(t, wheelFake(), Options{Now: fixedNow})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	key(t, m, "l", "j")
	x, y := m.rightX()+5, 1+m.rightHead()+2
	if h := m.hitTest(x, y); h.what != hitItem {
		t.Fatalf("榜单区应命中条目：%+v", h)
	}
	for i := 0; i < 5; i++ {
		wheel(m, x, y, tea.MouseButtonWheelDown)
	}
	if m.listCur != 1 || m.listOff != 15 {
		t.Fatalf("滚轮只滚视窗，选中不变：cur=%d off=%d", m.listCur, m.listOff)
	}
	if strings.Contains(plainView(m), "条目01") {
		t.Fatal("选中项可以滚出视野")
	}
	if !strings.Contains(previewText(m), "条目01") && m.pvKey != "k01" {
		t.Fatal("预览跟着选中项，不跟视窗")
	}
	// 悬停高亮对应指针下实际的行。
	if h := m.hitTest(x, y); !m.hovered(focusList, h.idx) || h.idx != 17 {
		t.Fatalf("悬停应对应滚动后的行：%+v", h)
	}
	// ↓ 从选中项继续移动，视窗拉回选中项。
	key(t, m, "j")
	if m.listCur != 2 || m.listOff > 2 || !strings.Contains(plainView(m), "条目02") {
		t.Fatalf("↓ 应从选中项继续：cur=%d off=%d", m.listCur, m.listOff)
	}
	// 来源栏滚轮同样不改选中。
	wheel(m, 2, 3, tea.MouseButtonWheelDown)
	if m.srcCur != 0 {
		t.Fatal("来源栏滚轮不改选中")
	}
}

func TestWheelPreview(t *testing.T) {
	m := start(t, wheelFake(), Options{Now: fixedNow})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	key(t, m, "l")
	waitInsight(t, m)
	py := 1 + m.rightHead() + m.listHeight() + 2
	if h := m.hitTest(m.rightX()+5, py); h.what != hitPreview {
		t.Fatalf("榜单下方应命中预览区：%+v", h)
	}
	// 分隔线也算预览区，不会滚榜单。
	if h := m.hitTest(m.rightX()+5, 1+m.rightHead()+m.listHeight()); h.what != hitPreview {
		t.Fatalf("分隔线：%+v", h)
	}
	w, h := m.previewBox()
	n := len(m.previewContent(w))
	wheel(m, m.rightX()+5, py, tea.MouseButtonWheelDown)
	if m.pvOff != wheelStep || m.listOff != 0 || m.listCur != 0 {
		t.Fatalf("预览区滚轮按行滚预览：pv=%d list=%d", m.pvOff, m.listOff)
	}
	for i := 0; i < 30; i++ {
		wheel(m, m.rightX()+5, py, tea.MouseButtonWheelDown)
	}
	if m.pvOff != n-(h-1) {
		t.Fatalf("到底停住不回绕：%d，期望 %d", m.pvOff, n-(h-1))
	}
	if body := m.previewBody(w+2, h); strings.Contains(strings.Join(body, ""), "↓ 更多") {
		t.Fatal("到底后不再提示更多")
	}
	for i := 0; i < 30; i++ {
		wheel(m, m.rightX()+5, py, tea.MouseButtonWheelUp)
	}
	if m.pvOff != 0 {
		t.Fatal("到顶停住")
	}
	// 空格翻页逻辑不变。
	key(t, m, " ")
	if m.pvOff != h-1 {
		t.Fatalf("空格仍整页翻：%d", m.pvOff)
	}
	// 点击预览不选中条目。
	m.Update(tea.MouseMsg{X: m.rightX() + 5, Y: py, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.listCur != 0 {
		t.Fatal("点击预览不选中条目")
	}

	// 单栏整栏预览：滚轮滚预览。
	m.Update(tea.WindowSizeMsg{Width: 48, Height: 16})
	key(t, m, "p")
	if hh := m.hitTest(5, 5); hh.what != hitPreview {
		t.Fatalf("整栏预览应命中预览：%+v", hh)
	}
	wheel(m, 5, 5, tea.MouseButtonWheelDown)
	if m.pvOff != wheelStep || m.listCur != 0 {
		t.Fatalf("整栏预览滚轮应滚预览：%d", m.pvOff)
	}
}

func TestWheelStrips(t *testing.T) {
	f := wheelFake()
	m := start(t, f, Options{Now: fixedNow})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	key(t, m, "j")
	settle(t, m)
	key(t, m, "l")
	// 子榜单条：横向平移，不切换子榜单。
	sub0, x0 := m.subIdx, m.rightX()
	before := m.stripSpans(x0)
	if before[len(before)-1].idx == 11 {
		t.Fatal("子榜单应放不下")
	}
	calls := len(f.listCalls)
	for i := 0; i < 3; i++ {
		wheel(m, x0+10, 2, tea.MouseButtonWheelDown)
	}
	after := m.stripSpans(x0)
	if m.subIdx != sub0 || len(f.listCalls) != calls || after[0].idx != before[0].idx+3 {
		t.Fatalf("滚轮应平移子榜单条：sub=%d first %d→%d", m.subIdx, before[0].idx, after[0].idx)
	}
	row := plainView(m)
	if l := strings.Split(row, "\n")[2]; !strings.Contains(l, "‹") || !strings.Contains(l, fmt.Sprintf("子榜单%02d", after[0].idx)) {
		t.Fatalf("‹ › 按新窗口显示：%q", l)
	}
	// 平移到头停住，最后一项可见。
	for i := 0; i < 20; i++ {
		wheel(m, x0+10, 2, tea.MouseButtonWheelDown)
	}
	sp := m.stripSpans(x0)
	if sp[len(sp)-1].idx != 11 || strings.Contains(strings.Split(plainView(m), "\n")[2], "›") {
		t.Fatalf("应停在最后：%+v", sp)
	}
	// 偏移后点击命中准确。
	target := sp[1]
	m.Update(tea.MouseMsg{X: target.x0 + 1, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	drain(t, m, nil)
	if m.subIdx != target.idx {
		t.Fatalf("点击应命中 %d，实际 %d", target.idx, m.subIdx)
	}
	if got := m.stripSpans(x0); got[0].idx != sp[0].idx {
		t.Fatal("点击后保持平移过的窗口")
	}
	// 键盘切换后窗口回到以选中项为中心。
	key(t, m, "tab")
	if m.subWin.start(m.subIdx, m.target.key()) >= 0 {
		t.Fatal("键盘切换后应对准选中项")
	}

	// 菜单栏：横向平移，不切换分类。
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	menu0 := m.menuIdx
	ms := m.menuSpans()
	wheel(m, 30, 0, tea.MouseButtonWheelDown)
	wheel(m, 30, 0, tea.MouseButtonWheelDown)
	ms2 := m.menuSpans()
	if m.menuIdx != menu0 || ms2[0].idx <= ms[0].idx {
		t.Fatalf("菜单栏应平移：%v → %v", ms[0], ms2[0])
	}
	if !strings.HasPrefix(strings.TrimLeft(strings.Split(plainView(m), "\n")[0], " "), "今日热榜  ‹") {
		t.Fatalf("菜单栏左侧应显示 ‹：%q", strings.Split(plainView(m), "\n")[0])
	}
	tg := ms2[len(ms2)-1]
	m.Update(tea.MouseMsg{X: tg.x0 + 1, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.menuIdx != tg.idx {
		t.Fatalf("点击菜单应命中 %d，实际 %d", tg.idx, m.menuIdx)
	}
	key(t, m, "1")
	if m.menuWin.start(m.menuIdx, "") >= 0 {
		t.Fatal("数字键切换后应对准选中分类")
	}
	// 横向滚轮按钮同样平移。
	wheel(m, 30, 0, tea.MouseButtonWheelRight)
	if m.menuWin.start(m.menuIdx, "") < 0 {
		t.Fatal("横向滚轮应平移菜单栏")
	}
	// 帮助打开时滚轮不做任何事。
	key(t, m, "?")
	off := m.menuWin
	wheel(m, 30, 0, tea.MouseButtonWheelRight)
	if m.menuWin != off {
		t.Fatal("帮助中滚轮保持现状")
	}
}
