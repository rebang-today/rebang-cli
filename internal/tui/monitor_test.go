package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/errs"
)

// monitorFake 在订阅里放“内容监控”与微博两个来源；45 条命中：偶数编号属于计划 7，奇数属于计划 3，编号能被 3 整除的已读。
func monitorFake() *fakeSource {
	f := newFake()
	f.follow = []api.Tab{
		{Key: "monitoring", Name: "内容监控", Type: "monitoring", Children: []api.SubTab{{Key: "monitoring:all", Name: "全部"}}},
		{Key: "weibo", Name: "微博", Children: []api.SubTab{{Key: "hot", Name: "热搜"}, {Key: "news", Name: "要闻"}}},
	}
	f.monPlans = []api.MonitorPlan{
		{ID: 7, Name: "英伟达", Keywords: []string{"英伟达"}, Status: api.MonitorActive},
		{ID: 3, Name: "苹果", Keywords: []string{"苹果"}, Status: api.MonitorPaused},
		{ID: 5, Name: "特斯拉", Keywords: []string{"特斯拉"}, Status: api.MonitorMembershipExpired},
	}
	f.monUnread = &api.MonitorUnread{All: 12, Plans: []api.MonitorPlanUnread{{PlanID: 7, UnreadCount: 3}, {PlanID: 3, UnreadCount: 2}}}
	now := fixedNow().Unix()
	for i := 0; i < 45; i++ {
		plan, name := int64(3), "苹果"
		if i%2 == 0 {
			plan, name = 7, "英伟达"
		}
		f.monHits = append(f.monHits, api.MonitorHit{
			ItemKey: fmt.Sprintf("%032x", i), Title: fmt.Sprintf("命中%02d", i), Description: "描述",
			SourceKey: "weibo", SourceName: "微博", WwwURL: fmt.Sprintf("https://example.com/h%d", i),
			MatchedAt: now - int64(i+3)*60, IsRead: i%3 == 0,
			PlanMatches: []api.MonitorPlanMatch{{PlanID: plan, PlanName: name, MatchedKeywords: []string{name, "GPU"}}},
		})
	}
	return f
}

func startMonitor(t *testing.T, f *fakeSource, opt Options) *model {
	t.Helper()
	opt.Now = fixedNow
	if opt.Web == "" {
		opt.Web = "https://web.example/"
	}
	m := start(t, f, opt)
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	key(t, m, "0", "l")
	return m
}

func TestMonitorList(t *testing.T) {
	f := monitorFake()
	m := startMonitor(t, f, Options{LoggedIn: true})
	if !m.inMonitor() || m.focus != focusList || len(m.items) != 30 {
		t.Fatalf("应显示命中列表第一页：monitor=%v items=%d", m.inMonitor(), len(m.items))
	}
	if q := f.monHitsCalls[0]; q.PageSize != 30 || q.Cursor != "" || q.PlanID != 0 || q.UnreadOnly {
		t.Fatalf("第一页查询：%+v", q)
	}
	v := plainView(m)
	for _, s := range []string{"内容监控 12", "全部 12", "英伟达 3", "苹果 2 已暂停", "特斯拉 会员过期", "30+ 条", "u 只看未读"} {
		if !strings.Contains(v, s) {
			t.Fatalf("缺少 %q\n%s", s, v)
		}
	}
	// 每行：未读圆点、标题、“来源 · 命中词”、右侧相对时间；已读的不显示圆点，宽度不变。
	l1, l0 := lineWith(v, "命中01"), lineWith(v, "命中00")
	if !strings.Contains(l1, "● ") || !strings.Contains(l1, "微博 · 苹果、GPU") || !strings.HasSuffix(strings.TrimRight(l1, " "), "4 分钟前") {
		t.Fatalf("未读行：%q", l1)
	}
	if strings.Contains(l0, "●") || strings.Index(l0, "命中00") != strings.Index(l1, "命中01") {
		t.Fatalf("已读行不显示圆点且对齐：\n%q\n%q", l0, l1)
	}
	// 暂停、会员过期的附注是灰色。
	strip := lineWith(m.View(), "全部 12")
	if !strings.Contains(strip, lipgloss.NewStyle().Foreground(colorDim).Render("已暂停")) {
		t.Fatalf("已暂停应为灰色：%q", strip)
	}
}

func TestMonitorPaging(t *testing.T) {
	f := monitorFake()
	m := startMonitor(t, f, Options{LoggedIn: true})
	for i := 0; i < 24; i++ {
		key(t, m, "j")
	}
	if len(f.monHitsCalls) != 1 {
		t.Fatalf("离末尾超过 5 条时不应翻页：%d", len(f.monHitsCalls))
	}
	key(t, m, "j") // 第 26 条，距末尾 4 条
	if len(f.monHitsCalls) != 2 || f.monHitsCalls[1].Cursor != "30" || len(m.items) != 45 || m.mon.hasMore {
		t.Fatalf("应用 next_cursor 加载下一页：calls=%+v items=%d", f.monHitsCalls, len(m.items))
	}
	if !strings.Contains(plainView(m), "45 条") {
		t.Fatal("加载完后不再显示 +")
	}
	key(t, m, "G")
	if len(f.monHitsCalls) != 2 {
		t.Fatal("没有更多时不应再请求")
	}
	// r 从第一页重新加载。
	key(t, m, "r")
	if q := f.monHitsCalls[len(f.monHitsCalls)-1]; q.Cursor != "" || len(m.items) != 30 {
		t.Fatalf("r 应从第一页加载：%+v items=%d", q, len(m.items))
	}
	// 自动刷新同样从第一页加载。
	n := len(f.monHitsCalls)
	_, cmd := m.Update(tickMsg{})
	drain(t, m, cmd)
	if len(f.monHitsCalls) != n+1 || f.monHitsCalls[n].Cursor != "" {
		t.Fatalf("自动刷新：%+v", f.monHitsCalls[n:])
	}

	// 下一页失败：提示错误、不再自动重试。
	f2 := monitorFake()
	m2 := startMonitor(t, f2, Options{LoggedIn: true})
	f2.monHitsErr = errs.Network(errors.New("refused"))
	key(t, m2, "G")
	key(t, m2, "k", "j")
	if len(f2.monHitsCalls) != 2 || !m2.isError || len(m2.items) != 30 {
		t.Fatalf("下一页失败：calls=%d err=%v", len(f2.monHitsCalls), m2.isError)
	}
}

func TestMonitorOpenMarksRead(t *testing.T) {
	f := monitorFake()
	var opened []string
	m := startMonitor(t, f, Options{LoggedIn: true, Open: func(u string) error { opened = append(opened, u); return nil }})
	key(t, m, "j") // 命中01，未读
	key(t, m, "enter")
	k1 := fmt.Sprintf("%032x", 1)
	if len(opened) != 1 || opened[0] != "https://example.com/h1" || len(f.monReadCalls) != 1 || f.monReadCalls[0] != k1 {
		t.Fatalf("回车应打开原文并标记已读：%v %v", opened, f.monReadCalls)
	}
	if m.items[1].unread || m.mon.unread.All != 11 || !strings.Contains(plainView(m), "全部 11") || !strings.Contains(plainView(m), "内容监控 11") {
		t.Fatalf("应在本地标为已读并更新未读数\n%s", plainView(m))
	}
	// 已读的再打开不重复调用。
	key(t, m, "enter")
	if len(opened) != 2 || len(f.monReadCalls) != 1 {
		t.Fatalf("已读条目不应再标记：%v", f.monReadCalls)
	}
	// read 接口失败不影响打开原文。
	f.monReadErr = errs.Network(errors.New("refused"))
	key(t, m, "j", "enter") // 命中02，未读
	if len(opened) != 3 || opened[2] != "https://example.com/h2" || m.items[2].unread || m.isError {
		t.Fatalf("read 失败时仍应打开原文：%v err=%v %q", opened, m.isError, m.message)
	}
	// 详情页回车同样标记已读。
	f.monReadErr = nil
	key(t, m, "j", "j", " ", "enter") // 命中04
	if m.det == nil || m.det.it.unread || f.monReadCalls[len(f.monReadCalls)-1] != fmt.Sprintf("%032x", 4) {
		t.Fatalf("详情页回车应标记已读：%v", f.monReadCalls)
	}
}

func TestMonitorUnreadToggle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tui-state.json")
	f := monitorFake()
	m := startMonitor(t, f, Options{LoggedIn: true, StatePath: path})
	key(t, m, "j", "j")
	key(t, m, "u")
	q := f.monHitsCalls[len(f.monHitsCalls)-1]
	if !q.UnreadOnly || q.Cursor != "" || m.listCur != 0 {
		t.Fatalf("u 应从第一页加载未读：%+v cur=%d", q, m.listCur)
	}
	for _, it := range m.items {
		if !it.unread {
			t.Fatalf("只看未读时出现已读：%+v", it)
		}
	}
	if v := plainView(m); !strings.Contains(v, "内容监控 · 只看未读") || !strings.Contains(v, "u 看全部") {
		t.Fatalf("标题与提示应显示只看未读\n%s", v)
	}
	m.SaveState()
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"monitor_unread": true`) {
		t.Fatalf("应保存只看未读：%s", b)
	}
	f2 := monitorFake()
	m2 := startMonitor(t, f2, Options{LoggedIn: true, StatePath: path})
	if !m2.mon.unreadOnly || !f2.monHitsCalls[0].UnreadOnly {
		t.Fatal("重启后应恢复只看未读")
	}
	key(t, m2, "u")
	m2.SaveState()
	b, _ = os.ReadFile(path)
	if strings.Contains(string(b), "monitor_unread") || f2.monHitsCalls[len(f2.monHitsCalls)-1].UnreadOnly {
		t.Fatalf("切回全部：%s", b)
	}
	// 普通榜单里 u 无动作。
	key(t, m2, "h", "j")
	settle(t, m2)
	key(t, m2, "l")
	n := len(f2.monHitsCalls)
	key(t, m2, "u")
	if m2.mon.unreadOnly || len(f2.monHitsCalls) != n {
		t.Fatal("普通榜单里 u 不应生效")
	}
	if strings.Contains(plainView(m2), "只看未读") {
		t.Fatal("普通榜单不显示 u 提示")
	}
}

func TestMonitorPlanSubs(t *testing.T) {
	f := monitorFake()
	m := startMonitor(t, f, Options{LoggedIn: true})
	key(t, m, "tab")
	q := f.monHitsCalls[len(f.monHitsCalls)-1]
	if q.PlanID != 7 || q.Cursor != "" || m.subIdx != 1 {
		t.Fatalf("→ 应切到第一个计划：%+v", q)
	}
	for _, it := range m.items {
		if it.plans[0].PlanID != 7 {
			t.Fatalf("应只显示计划 7：%+v", it)
		}
	}
	key(t, m, "tab")
	if q := f.monHitsCalls[len(f.monHitsCalls)-1]; q.PlanID != 3 {
		t.Fatalf("%+v", q)
	}
	// 计划没有命中内容：提示，不崩溃。
	key(t, m, "tab")
	if len(m.items) != 0 || !strings.Contains(plainView(m), "还没有命中内容") {
		t.Fatalf("特斯拉没有命中\n%s", plainView(m))
	}
	// 计划被删除：回到全部。
	f.monPlans = f.monPlans[:1]
	key(t, m, "r")
	if m.subIdx != 0 || f.monHitsCalls[len(f.monHitsCalls)-1].PlanID != 0 {
		t.Fatalf("计划被删除后应回到全部：sub=%d", m.subIdx)
	}
}

func TestMonitorDetail(t *testing.T) {
	f := monitorFake()
	f.insights = map[string]*api.Insight{fmt.Sprintf("%032x", 1): {SummaryShort: "命中的一句话总结"}}
	f.shorts = map[string]string{fmt.Sprintf("%032x", 1): "命中的一句话总结"}
	m := startMonitor(t, f, Options{LoggedIn: true})
	if v := plainView(m); !strings.Contains(lineWith(v, "命中01"), insightMark+"命中01") {
		t.Fatalf("有 AI 总结的命中应显示标记\n%s", v)
	}
	key(t, m, "j", " ")
	v := plainView(m)
	for _, s := range []string{"命中01", "微博 · 4 分钟前", "命中", "• 苹果：苹果、GPU", "AI 总结", "命中的一句话总结"} {
		if !strings.Contains(v, s) {
			t.Fatalf("详情缺少 %q\n%s", s, v)
		}
	}
	if strings.Index(v, "苹果：苹果") > strings.Index(v, "AI 总结") {
		t.Fatal("命中区块应在 AI 总结之前")
	}
	// 预览照常工作。
	key(t, m, "esc")
	if v := plainView(m); !strings.Contains(v, "AI 总结 命中的一句话总结") {
		t.Fatalf("预览应显示 AI 总结\n%s", v)
	}
}

func TestMonitorEmptyStates(t *testing.T) {
	// 没有计划。
	f := monitorFake()
	f.monPlans = nil
	m := startMonitor(t, f, Options{LoggedIn: true})
	if v := plainView(m); !strings.Contains(v, "还没有监控计划，可在网站创建：https://web.example/monitoring") || len(f.monHitsCalls) != 0 {
		t.Fatalf("没有计划时应提示去网站创建\n%s", v)
	}
	// 未登录：沿用订阅的登录提示，不请求监控接口。
	f2 := monitorFake()
	m2 := startMonitor(t, f2, Options{})
	if v := plainView(m2); !strings.Contains(v, "登录后查看订阅") || f2.monUnreadN != 0 || len(f2.monHitsCalls) != 0 {
		t.Fatalf("未登录：\n%s", v)
	}
	// 401：标记登录失效。
	f3 := monitorFake()
	f3.monErr = errs.Unauthorized("")
	m3 := startMonitor(t, f3, Options{LoggedIn: true})
	if !m3.expired {
		t.Fatal("401 应标记登录失效")
	}
}

func TestMonitorRelTime(t *testing.T) {
	now := time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC)
	cases := map[time.Duration]string{
		10 * time.Second: "刚刚", 3 * time.Minute: "3 分钟前", 2 * time.Hour: "2 小时前",
		16 * time.Hour: "昨天 23:00", 40 * time.Hour: "11月11日 23:00", 400 * 24 * time.Hour: "2025年10月9日",
		-time.Hour: "刚刚",
	}
	for d, want := range cases {
		if got := relTime(now.Add(-d).Unix(), now); got != want {
			t.Errorf("%v → %q want %q", d, got, want)
		}
	}
	if relTime(0, now) != "" {
		t.Fatal("0 应为空")
	}
}

func TestMonitorSmallSizes(t *testing.T) {
	f := monitorFake()
	m := startMonitor(t, f, Options{LoggedIn: true})
	sizes := [][2]int{{110, 30}, {56, 24}, {55, 12}, {40, 10}, {39, 10}, {20, 5}, {1, 1}, {200, 60}}
	for _, open := range []bool{false, true} {
		if open {
			key(t, m, " ")
		}
		for _, sz := range sizes {
			m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
			v := m.View()
			for _, l := range strings.Split(v, "\n") {
				if lipgloss.Width(l) > sz[0] {
					t.Fatalf("%dx%d 溢出：%q", sz[0], sz[1], l)
				}
			}
			key(t, m, "j", "u", "tab", "G")
		}
	}
}
