package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/cli"
)

// stateFake 的科技菜单也有 Tab，便于测试跨菜单的记忆。
func stateFake() *fakeSource {
	f := newFake()
	f.data[1].tabs[1].Children = append(f.data[1].tabs[1].Children, api.SubTab{Key: "ent", Name: "文娱"})
	f.data[2] = &menuData{tabs: []api.Tab{{Key: "ithome", Name: "IT之家"}, {Key: "ifanr", Name: "爱范儿"}}}
	f.lists["|weibo|ent"] = []cli.ListItem{{Title: "微博文娱", URL: "https://example.com/w3"}}
	f.lists["|ithome|"] = []cli.ListItem{{Title: "IT之家文章", URL: "https://example.com/i1"}}
	f.lists["|ifanr|"] = []cli.ListItem{{Title: "爱范儿文章", URL: "https://example.com/f1"}}
	return f
}

// quit 模拟退出：Run 在程序结束后调用 SaveState。
func quit(m *model) { m.SaveState() }

func TestStateRestoresAcrossMenus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", stateFileName)
	f := stateFake()
	m := start(t, f, Options{StatePath: path})
	key(t, m, "j") // 综合 › 微博
	settle(t, m)
	key(t, m, "tab", "tab") // 文娱
	key(t, m, "2", "j")     // 科技 › 爱范儿
	settle(t, m)
	if m.target.tab != "ifanr" {
		t.Fatalf("target=%+v", m.target)
	}
	quit(m)

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("状态文件权限应为 0600，got %v", fi.Mode().Perm())
	}
	if di, _ := os.Stat(filepath.Dir(path)); runtime.GOOS != "windows" && di.Mode().Perm() != 0o700 {
		t.Fatalf("目录权限应为 0700，got %v", di.Mode().Perm())
	}
	var raw map[string]any
	b, _ := os.ReadFile(path)
	if json.Unmarshal(b, &raw) != nil || raw["version"] != float64(stateVersion) {
		t.Fatalf("状态文件应带 version\n%s", b)
	}

	m2 := start(t, stateFake(), Options{StatePath: path})
	if m2.menuIdx != 2 || m2.target.tab != "ifanr" || m2.items[0].title != "爱范儿文章" {
		t.Fatalf("应回到科技 › 爱范儿，menu=%d target=%+v", m2.menuIdx, m2.target)
	}
	key(t, m2, "1")
	if m2.target.tab != "weibo" || m2.target.subTab != "ent" || m2.items[0].title != "微博文娱" {
		t.Fatalf("综合应回到微博·文娱，target=%+v", m2.target)
	}
	if m2.listCur != 0 || m2.searchQ != "" {
		t.Fatal("不应恢复列表光标与搜索词")
	}
}

func TestStateFallsBackWhenKeysGone(t *testing.T) {
	path := filepath.Join(t.TempDir(), stateFileName)
	m := start(t, stateFake(), Options{StatePath: path})
	key(t, m, "j")
	settle(t, m)
	key(t, m, "tab", "tab") // 微博·文娱
	key(t, m, "2", "j")     // 科技 › 爱范儿
	settle(t, m)
	quit(m)

	// 子榜单“文娱”下线：微博回到默认（第一个）子榜单。
	f := stateFake()
	f.data[1].tabs[1].Children = f.data[1].tabs[1].Children[:2]
	m2 := start(t, f, Options{StatePath: path})
	key(t, m2, "1")
	if m2.target.tab != "weibo" || m2.target.subTab != "hot" {
		t.Fatalf("子榜单消失应回退到第一个，target=%+v", m2.target)
	}

	// 来源“爱范儿”下线：科技回到第一个来源。
	f = stateFake()
	f.data[2].tabs = f.data[2].tabs[:1]
	m3 := start(t, f, Options{StatePath: path})
	if m3.menuIdx != 2 || m3.target.tab != "ithome" {
		t.Fatalf("来源消失应回退到第一个来源，menu=%d target=%+v", m3.menuIdx, m3.target)
	}

	// 菜单“科技”被删：回到第一个服务端菜单。
	f = stateFake()
	f.menus = f.menus[:1]
	m4 := start(t, f, Options{StatePath: path})
	if m4.menuIdx != 1 || m4.target.tab != "weibo" || m4.target.subTab != "ent" {
		t.Fatalf("菜单消失应回到第一个服务端菜单，menu=%d target=%+v", m4.menuIdx, m4.target)
	}
}

func TestStateCorruptOrUnknown(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"corrupt": "{not json",
		"version": `{"version": 99, "menu": 2}`,
		"empty":   "",
	} {
		path := filepath.Join(dir, name+".json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		m := start(t, stateFake(), Options{StatePath: path})
		if m.menuIdx != 1 || m.target.tab != "zhihu" {
			t.Fatalf("%s：应当作没有记忆，menu=%d target=%+v", name, m.menuIdx, m.target)
		}
	}
	// 目录不可写时照常使用。
	m := start(t, stateFake(), Options{StatePath: filepath.Join(dir, "corrupt.json", "x", stateFileName)})
	key(t, m, "2")
	quit(m)
	if m.target.tab != "ithome" {
		t.Fatalf("target=%+v", m.target)
	}
}

func TestStateFollowMenuLoggedOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), stateFileName)
	f := stateFake()
	f.follow = []api.Tab{{Key: "weibo", Name: "微博", Children: []api.SubTab{{Key: "hot", Name: "热搜"}, {Key: "news", Name: "要闻"}}}}
	m := start(t, f, Options{StatePath: path, LoggedIn: true})
	key(t, m, "0")
	quit(m)

	// 未登录时不回到空的订阅页，停在第一个服务端菜单（综合）。
	m2 := start(t, stateFake(), Options{StatePath: path})
	if m2.menuIdx != 1 || m2.inFollowMenu() {
		t.Fatalf("未登录应停在综合，menu=%d", m2.menuIdx)
	}
	// 手动切到订阅仍显示登录提示。
	key(t, m2, "0")
	if v := m2.View(); !containsAll(v, "登录后查看订阅") {
		t.Fatalf("未登录应显示登录提示\n%s", v)
	}
}

func TestStateDebouncedSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), stateFileName)
	m := start(t, stateFake(), Options{StatePath: path})
	if _, err := os.Stat(path); err == nil {
		t.Fatal("没有切换时不应写文件")
	}
	before := m.saveSeq
	key(t, m, "2")
	if m.saveSeq == before {
		t.Fatal("切换菜单后应安排写入")
	}
	seq := m.saveSeq
	key(t, m, "j")
	if m.saveSeq == seq {
		t.Fatal("再次切换应重新计时")
	}
	m.Update(saveMsg{seq}) // 过期的计时不写入
	if _, err := os.Stat(path); err == nil {
		t.Fatal("过期的计时不应写文件")
	}
	m.Update(saveMsg{m.saveSeq})
	s := loadPosState(path)
	if s.Menu == nil || *s.Menu != 2 || s.Sources["2"] != "t:ifanr" {
		t.Fatalf("应写入当前位置，got %+v", s)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, x := range subs {
		if !strings.Contains(s, x) {
			return false
		}
	}
	return true
}

func TestStateNarrowPick(t *testing.T) {
	path := filepath.Join(t.TempDir(), stateFileName)
	m := start(t, stateFake(), Options{StatePath: path})
	m.Update(tea.WindowSizeMsg{Width: 48, Height: 12})
	key(t, m, "left", "j")
	if !m.picking() || m.srcCur != 1 {
		t.Fatal("应打开来源列表并移动光标")
	}
	// 单栏来源列表里 ↑↓ 即切换来源，不需要确认。
	quit(m)
	if s := loadPosState(path); s.Sources["1"] != "t:weibo" {
		t.Fatalf("移动后的来源应保存，got %+v", s)
	}
	if !m.picking() || m.srcCur != 1 {
		t.Fatal("保存不应改变界面状态")
	}
}
