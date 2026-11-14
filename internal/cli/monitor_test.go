package cli

import (
	"strings"
	"testing"

	"github.com/rebang-today/rebang-cli/internal/credentials"
	"github.com/rebang-today/rebang-cli/internal/errs"
)

func TestMonitorPlans(t *testing.T) {
	h := newHarness(t)
	t.Setenv(credentials.EnvToken, "rb_cli_valid")

	out, stderr, code := h.run(false, "monitor", "plans")
	if code != 0 {
		t.Fatal(code, stderr)
	}
	var res MonitorPlansResult
	decode(t, out, &res)
	if res.UnreadAll != 12 || len(res.Plans) != 2 {
		t.Fatalf("%+v", res)
	}
	p := res.Plans[0]
	if p.ID != 7 || p.Name != "英伟达" || strings.Join(p.Keywords, ",") != "英伟达,NVIDIA" || p.Status != "active" || p.UnreadCount != 3 || res.Plans[1].UnreadCount != 0 {
		t.Fatalf("%+v", res.Plans)
	}

	out, _, code = h.run(true, "monitor", "plans")
	for _, s := range []string{"ID", "名称", "关键词", "状态", "未读", "英伟达、NVIDIA", "已暂停", "监控中", "rebang monitor hits --plan"} {
		if code != 0 || !strings.Contains(out, s) {
			t.Fatalf("table 缺少 %q\n%s", s, out)
		}
	}
	out, _, _ = h.run(false, "monitor", "plans", "-o", "md")
	if !strings.Contains(out, "- 英伟达（ID 7，监控中，未读 3）：英伟达、NVIDIA") {
		t.Fatal(out)
	}

	// 未登录：不发请求，退出码 3。
	t.Setenv(credentials.EnvToken, "")
	n := len(h.api.requests)
	if _, _, code := h.run(false, "monitor", "plans"); code != errs.ExitAuth || len(h.api.requests) != n {
		t.Fatal(code)
	}
	// 令牌失效：服务端 401，退出码 3。
	t.Setenv(credentials.EnvToken, "rb_cli_bad")
	if _, _, code := h.run(false, "monitor", "plans"); code != errs.ExitAuth {
		t.Fatal(code)
	}
}

func TestMonitorHits(t *testing.T) {
	h := newHarness(t)
	t.Setenv(credentials.EnvToken, "rb_cli_valid")

	// -n 40 跨两页：30 + 10，还有更多。
	n := len(h.api.requests)
	out, stderr, code := h.run(false, "monitor", "hits", "-n", "40")
	if code != 0 {
		t.Fatal(code, stderr)
	}
	var res MonitorHitsResult
	decode(t, out, &res)
	if len(res.Items) != 40 || !res.HasMore || res.Items[39].Title != "命中39" {
		t.Fatalf("%d %v", len(res.Items), res.HasMore)
	}
	var cursors []string
	for _, r := range h.api.requests[n:] {
		if r.URL.Path == "/v1/monitoring/hits" {
			cursors = append(cursors, r.URL.Query().Get("cursor")+"/"+r.URL.Query().Get("page_size"))
		}
	}
	if strings.Join(cursors, ",") != "/30,30/30" {
		t.Fatalf("应按 cursor 翻页：%v", cursors)
	}
	it := res.Items[1]
	if it.ItemKey != "00000000000000000000000000000001" || it.Source != "weibo" || it.SourceName != "微博" || it.URL != "https://example.com/h1" ||
		it.MobileURL != "https://m.example.com/h1" || it.IsRead || it.MatchedAt == "" || len(it.Matches) != 1 || it.Matches[0].PlanName != "苹果" || it.Matches[0].MatchedKeywords[0] != "苹果" {
		t.Fatalf("%+v", it)
	}
	if !res.Items[0].IsRead {
		t.Fatal("0 号应为已读")
	}

	// 取完全部：has_more 为 false。
	out, _, _ = h.run(false, "monitor", "hits", "-n", "100")
	res = MonitorHitsResult{}
	decode(t, out, &res)
	if len(res.Items) != 45 || res.HasMore {
		t.Fatalf("%d %v", len(res.Items), res.HasMore)
	}

	// --plan 与 --unread 传给服务端；只读，不调用 read 接口。
	n = len(h.api.requests)
	out, _, _ = h.run(false, "monitor", "hits", "--plan", "7", "--unread", "-n", "5")
	res = MonitorHitsResult{}
	decode(t, out, &res)
	q := h.api.requests[n].URL.Query()
	if q.Get("plan_id") != "7" || q.Get("read_status") != "unread" || q.Get("page_size") != "5" || res.PlanID != 7 || !res.UnreadOnly {
		t.Fatalf("%v %+v", q, res)
	}
	for _, it := range res.Items {
		if it.IsRead || it.Matches[0].PlanID != 7 {
			t.Fatalf("%+v", it)
		}
	}
	for _, r := range h.api.requests[n:] {
		if strings.Contains(r.URL.Path, "/read") {
			t.Fatal("不应标记已读")
		}
	}

	// table 与 md。
	out, _, _ = h.run(true, "monitor", "hits", "-n", "3")
	for _, s := range []string{"未读", "标题", "来源", "命中", "命中时间", "item_key", "命中01", "苹果：苹果", "●", "00000000000000000000000000000002", "还有更多"} {
		if !strings.Contains(out, s) {
			t.Fatalf("table 缺少 %q\n%s", s, out)
		}
	}
	out, _, _ = h.run(false, "monitor", "hits", "-n", "2", "-o", "md")
	if !strings.Contains(out, "2. [命中01](https://example.com/h1)（未读） — 微博") || !strings.Contains(out, "item_key `00000000000000000000000000000001`") {
		t.Fatal(out)
	}
}

func TestMonitorHitsArgs(t *testing.T) {
	h := newHarness(t)
	t.Setenv(credentials.EnvToken, "rb_cli_valid")
	for _, args := range [][]string{{"-n", "0"}, {"-n", "101"}, {"--plan", "abc"}, {"--plan", "0"}, {"extra"}} {
		n := len(h.api.requests)
		if _, _, code := h.run(false, append([]string{"monitor", "hits"}, args...)...); code != errs.ExitUsage || len(h.api.requests) != n {
			t.Fatalf("%v → %d", args, code)
		}
	}
	t.Setenv(credentials.EnvToken, "")
	if _, _, code := h.run(false, "monitor", "hits"); code != errs.ExitAuth {
		t.Fatal(code)
	}
}
