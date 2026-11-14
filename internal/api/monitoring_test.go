package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rebang-today/rebang-cli/internal/errs"
)

func TestMonitorEndpoints(t *testing.T) {
	var got []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r)
		if r.Header.Get("Authorization") != "Bearer t" {
			t.Errorf("应带令牌：%q", r.Header.Get("Authorization"))
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/monitoring/plans":
			w.Write([]byte(`{"code":200,"msg":"ok","data":[{"id":7,"name":"英伟达","keywords":["英伟达","NVIDIA"],"status":"active","notification":{"enabled":true}},{"id":3,"name":"苹果","keywords":["苹果"],"status":"paused"}]}`))
		case "GET /v1/monitoring/unread-counts":
			ok(w, map[string]any{"all": 12, "plans": []map[string]any{{"plan_id": 7, "unread_count": 3}}})
		case "GET /v1/monitoring/hits":
			ok(w, map[string]any{"items": []map[string]any{{
				"item_key": "0123456789abcdef0123456789abcdef", "title": "标题", "source_name": "微博", "www_url": "", "mobile_url": "https://m.example.com/1",
				"matched_at": 1763000000, "is_read": false,
				"plan_matches": []map[string]any{{"plan_id": 7, "plan_name": "英伟达", "matched_keywords": []string{"英伟达", "GPU"}}, {"plan_id": 9, "plan_name": "显卡", "matched_keywords": []string{"GPU"}}},
			}}, "next_cursor": "c2", "has_more": true, "revision_at": 1, "revision_id": 2, "new_count": 0})
		case "POST /v1/monitoring/read":
			ok(w, map[string]any{"all": 11, "plans": []map[string]any{{"plan_id": 7, "unread_count": 2}}})
		default:
			t.Errorf("意外的请求 %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "t", InvocationTUI)
	ctx := context.Background()

	plans, err := c.MonitorPlans(ctx)
	if err != nil || len(plans) != 2 || plans[0].ID != 7 || plans[0].Keywords[1] != "NVIDIA" || plans[1].Status != MonitorPaused {
		t.Fatalf("%+v %v", plans, err)
	}
	u, err := c.MonitorUnreadCounts(ctx)
	if err != nil || u.All != 12 || u.Plan(7) != 3 || u.Plan(3) != 0 {
		t.Fatalf("%+v %v", u, err)
	}
	if (*MonitorUnread)(nil).Plan(1) != 0 {
		t.Fatal("nil 应返回 0")
	}

	hits, err := c.MonitorHitsPage(ctx, MonitorHitsQuery{PlanID: 7, Cursor: "c1", UnreadOnly: true, PageSize: 99})
	if err != nil || len(hits.Items) != 1 || hits.NextCursor != "c2" || !hits.HasMore {
		t.Fatalf("%+v %v", hits, err)
	}
	h := hits.Items[0]
	if h.URL() != "https://m.example.com/1" || strings.Join(h.MatchedKeywords(), ",") != "英伟达,GPU" || h.MatchedAt != 1763000000 {
		t.Fatalf("%+v", h)
	}
	q := got[len(got)-1].URL.Query()
	want := url.Values{"page_size": {"30"}, "plan_id": {"7"}, "cursor": {"c1"}, "read_status": {"unread"}}
	if q.Encode() != want.Encode() {
		t.Fatalf("查询参数 %s", q.Encode())
	}
	// 默认条件：只有 page_size。
	if _, err := c.MonitorHitsPage(ctx, MonitorHitsQuery{}); err != nil {
		t.Fatal(err)
	}
	if q := got[len(got)-1].URL.RawQuery; q != "page_size=30" {
		t.Fatalf("默认查询 %s", q)
	}

	u, err = c.MonitorMarkRead(ctx, "0123456789abcdef0123456789abcdef")
	if err != nil || u.All != 11 || u.Plan(7) != 2 {
		t.Fatalf("%+v %v", u, err)
	}
	if r := got[len(got)-1]; r.Header.Get("Content-Type") != "application/json" {
		t.Fatal(r.Header)
	}
}

func TestMonitorReadBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) != 1 || body["item_key"] != "abc" {
			t.Errorf("请求体 %v %v", body, err)
		}
		ok(w, map[string]any{"all": 0, "plans": []any{}})
	}))
	defer srv.Close()
	if _, err := New(srv.URL, "t", InvocationPipe).MonitorMarkRead(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
}

func TestMonitorErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fail(w, 401, 1004, "unauthorized")
	}))
	defer srv.Close()
	c := New(srv.URL, "bad", InvocationPipe)
	_, err := c.MonitorPlans(context.Background())
	if errs.ExitCode(MapError(err)) != errs.ExitAuth {
		t.Fatalf("401 应映射为未登录：%v", err)
	}
	_, err = c.MonitorHitsPage(context.Background(), MonitorHitsQuery{})
	if errs.ExitCode(MapError(err)) != errs.ExitAuth {
		t.Fatal(err)
	}
}
