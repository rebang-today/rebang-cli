package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rebang-today/rebang-cli/internal/cli"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/golden"
)

func fixedNow() time.Time { return time.Date(2026, 11, 13, 9, 30, 0, 0, time.UTC) }

func snapshot(m *model, w, h int) string {
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	lines := strings.Split(m.View(), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(golden.StripANSI(l), " ")
	}
	return strings.Join(lines, "\n") + "\n"
}

// TestViewGolden 覆盖常见宽度与状态：双栏、窄屏、帮助、搜索无结果、加载失败。
func TestViewGolden(t *testing.T) {
	var b strings.Builder
	section := func(name, s string) { b.WriteString("=== " + name + "\n" + s) }

	f := newFake()
	m := start(t, f, Options{Now: fixedNow})
	section("双栏·左栏焦点 90x10", snapshot(m, 90, 10))
	key(t, m, "l")
	section("双栏·列表焦点 90x10", snapshot(m, 90, 10))
	key(t, m, "h", "j", "enter", "tab")
	section("子榜单条 90x10", snapshot(m, 90, 10))
	section("单栏·榜单 48x10", snapshot(m, 48, 10))
	key(t, m, "left")
	section("单栏·来源列表 48x10", snapshot(m, 48, 10))
	key(t, m, "right")
	section("窗口太小 39x10", snapshot(m, 39, 10))
	key(t, m, "2")
	section("切换分类 90x10", snapshot(m, 90, 10))
	key(t, m, "0")
	section("订阅·未登录 90x10", snapshot(m, 90, 10))

	fe := newFake()
	fe.lists["|zhihu|"] = nil
	m = start(t, fe, Options{Now: fixedNow})
	section("空列表 80x10", snapshot(m, 80, 10))

	m = start(t, f, Options{Now: fixedNow})
	key(t, m, "?")
	section("帮助 80x18", snapshot(m, 80, 18))

	f2 := newFake()
	f2.search = []cli.SearchResultItem{}
	m = start(t, f2, Options{Now: fixedNow})
	key(t, m, "/", "x", "enter")
	section("搜索无结果 80x10", snapshot(m, 80, 10))

	f3 := newFake()
	f3.listErr = errs.Network(errors.New("dial tcp: refused"))
	m = start(t, f3, Options{Now: fixedNow})
	section("加载失败 80x10", snapshot(m, 80, 10))

	fi := insightFake()
	m = start(t, fi, Options{Now: fixedNow})
	key(t, m, "l")
	section("下方预览 100x26", snapshot(m, 100, 26))
	waitInsight(t, m)
	section("下方预览·完整总结 140x30", snapshot(m, 140, 30))
	key(t, m, " ")
	section("下方预览·翻页 140x30", snapshot(m, 140, 30))
	section("单栏·整栏预览 48x16", func() string {
		m.Update(tea.WindowSizeMsg{Width: 48, Height: 16})
		key(t, m, "p")
		return snapshot(m, 48, 16)
	}())

	golden.Assert(t, "views", b.String())
}

func TestHoverHighlight(t *testing.T) {
	m := start(t, newFake(), Options{Now: fixedNow})
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 10})
	lw := m.leftWidth()
	// 悬停在右栏第二条（第 1 行是标题，列表从第 2 行开始）。
	m.Update(tea.MouseMsg{X: lw + 5, Y: 3, Action: tea.MouseActionMotion, Button: tea.MouseButtonNone})
	if !m.hovered(focusList, 1) {
		t.Fatalf("应悬停在第 2 条：%+v", m.hoverIdx)
	}
	// 移出正文区域后取消悬停。
	m.Update(tea.MouseMsg{X: lw + 5, Y: 0, Action: tea.MouseActionMotion, Button: tea.MouseButtonNone})
	if m.hoverOK {
		t.Fatal("移到标题栏应取消悬停")
	}
	// 悬停不改变选中项。
	if m.listCur != 0 {
		t.Fatal(m.listCur)
	}
}

func TestSpinnerOnlyWhileLoading(t *testing.T) {
	m := start(t, newFake(), Options{Now: fixedNow})
	m.loading = true
	_, cmd := m.Update(spinMsg{})
	if cmd == nil {
		t.Fatal("加载中应继续动画")
	}
	m.loading = false
	if _, cmd := m.Update(spinMsg{}); cmd != nil || m.spinning {
		t.Fatal("加载结束后应停止动画")
	}
}

func TestTinyWindowsDoNotPanic(t *testing.T) {
	sizes := [][2]int{{0, 0}, {1, 1}, {4, 1}, {8, 2}, {12, 3}, {19, 4}, {20, 5}, {39, 10}, {40, 9}, {40, 10}, {48, 12}, {55, 10}, {56, 10}, {110, 30}, {129, 24}, {130, 16}, {130, 15}, {140, 30}, {200, 50}}
	for _, logged := range []bool{false, true} {
		m := start(t, newFake(), Options{LoggedIn: logged})
		for _, sz := range sizes {
			m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
			check := func(step string) {
				t.Helper()
				v := m.View()
				lines := strings.Split(v, "\n")
				if sz[1] > 0 && len(lines) > sz[1] {
					t.Fatalf("%dx%d %s：%d 行超过高度", sz[0], sz[1], step, len(lines))
				}
				for _, l := range lines {
					if w := lipgloss.Width(l); w > sz[0] {
						t.Fatalf("%dx%d %s：行宽 %d 超过终端宽度\n%q", sz[0], sz[1], step, w, l)
					}
				}
			}
			check("初始")
			for _, k := range []string{"down", "right", "2", "tab", "left", "down", "esc", "0", "?", "?", "1", "l", " ", "G", "j", "k", "g", "?", " ", "p", "p", "shift+tab"} {
				key(t, m, k)
				check(k)
			}
			for x := 0; x < sz[0]+2; x += 3 {
				for y := 0; y < sz[1]+1; y++ {
					m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion})
				}
			}
			check("鼠标")
		}
	}
}

func TestTooSmallHintAndRecover(t *testing.T) {
	m := start(t, newFake(), Options{})
	key(t, m, "j")
	m.Update(tea.WindowSizeMsg{Width: 39, Height: 10})
	if v := m.View(); !strings.Contains(v, "窗口太小，至少需要 40×10") || strings.Contains(v, "知乎") {
		t.Fatalf("39x10 低于最小宽度应只显示提示\n%s", v)
	}
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 9})
	if v := m.View(); !strings.Contains(v, "窗口太小") {
		t.Fatalf("40x9 低于最小高度应只显示提示\n%s", v)
	}
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	if v := m.View(); strings.Contains(v, "窗口太小") || !strings.Contains(v, "微博") {
		t.Fatalf("40x10 应正常显示\n%s", v)
	}
	m.Update(tea.WindowSizeMsg{Width: 6, Height: 2})
	if v := m.View(); strings.Contains(v, "\n\n\n") || !strings.Contains(v, "窗口") {
		t.Fatalf("很窄时提示应截断\n%q", v)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	if v := m.View(); !strings.Contains(v, "微博") || m.srcCur != 1 {
		t.Fatalf("放大后应恢复原界面与状态\n%s", v)
	}
	m.Update(tea.WindowSizeMsg{Width: 4, Height: 1})
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd == nil {
		t.Fatal("窗口太小时 q 仍应退出")
	}
}

func TestThemeColorsMatchWebsite(t *testing.T) {
	if colorAccent.Light != "#2C5CF5" || colorAccent.Dark != "#7AA2F7" {
		t.Fatalf("强调色应为网站蓝：%+v", colorAccent)
	}
	if styleRankTop.GetForeground() != lipgloss.TerminalColor(colorRankTop) || colorRankTop.Dark != "#F7670D" {
		t.Fatal("前 3 名序号应单独使用网站橙色，不跟随强调色")
	}
}
