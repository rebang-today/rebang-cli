package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
	section("双栏 90x10", snapshot(m, 90, 10))
	section("窄屏 50x6", snapshot(m, 50, 6))
	section("极窄 30x5", snapshot(m, 30, 5))

	m = start(t, f, Options{Now: fixedNow})
	key(t, m, "?")
	section("帮助 80x18", snapshot(m, 80, 18))

	f2 := newFake()
	f2.search = []cli.SearchResultItem{}
	m = start(t, f2, Options{Now: fixedNow})
	key(t, m, "/", "x", "enter")
	section("搜索无结果 80x8", snapshot(m, 80, 8))

	f3 := newFake()
	f3.listErr = errs.Network(errors.New("dial tcp: refused"))
	m = start(t, f3, Options{Now: fixedNow})
	section("加载失败 80x8", snapshot(m, 80, 8))

	golden.Assert(t, "views", b.String())
}

func TestHoverHighlight(t *testing.T) {
	m := start(t, newFake(), Options{Now: fixedNow})
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 10})
	lw := m.leftWidth()
	// 悬停在右栏第二行。
	m.Update(tea.MouseMsg{X: lw + 5, Y: 2, Action: tea.MouseActionMotion, Button: tea.MouseButtonNone})
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
