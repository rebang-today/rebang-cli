package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rebang-today/rebang-cli/internal/buildinfo"
	"github.com/rebang-today/rebang-cli/internal/config"
	"github.com/rebang-today/rebang-cli/internal/credentials"
	"github.com/rebang-today/rebang-cli/internal/errs"
)

// fakeAPI 模拟服务端 /v1 接口。
type fakeAPI struct {
	t        *testing.T
	mu       sync.Mutex
	requests []*http.Request
	vip      bool
	token    string // 有效 Token
	polls    int
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r)
	f.mu.Unlock()
	ok := func(data any) {
		json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "success", "data": data})
	}
	fail := func(status, code int, msg string) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{"code": code, "msg": msg})
	}
	authed := r.Header.Get("Authorization") == "Bearer "+f.token && f.token != ""
	switch r.URL.Path {
	case "/v1/menus":
		ok(map[string]any{"menus": []map[string]any{{"id": 1, "menu_name": "综合", "menu_key": "all"}, {"id": 2, "menu_name": "科技", "menu_key": "tech"}}})
	case "/v1/tabs/menu":
		if r.URL.Query().Get("menu_id") != "2" {
			fail(400, 1001, "params error")
			return
		}
		ok(map[string]any{"tab_info": []map[string]any{{"id": 9, "key": "github", "name": "GitHub", "child": []map[string]any{{"id": 10, "key": "daily", "name": "日榜"}}}}})
	case "/v1/tabs/children":
		ok(map[string]any{"tab_key": r.URL.Query().Get("tab_key"), "sub_tabs": []map[string]any{{"key": "daily", "name": "日榜"}}})
	case "/v1/nodes/menu":
		ok(map[string]any{"nodes": []map[string]any{{"node_id": "n-weibo", "name": "微博热搜", "subtitle": "实时"}}})
	case "/v1/latest_list":
		q := r.URL.Query()
		if q.Get("node_id") == "locked" {
			ok(map[string]any{"list": []any{}, "content_access": map[string]any{"mode": "locked", "reason": "membership_required"}})
			return
		}
		page := q.Get("page")
		items := []map[string]any{}
		start := 0
		if page == "2" {
			if q.Get("snapshot_id") != "snap" {
				fail(400, 1001, "params")
				return
			}
			start = 50
		}
		for i := 0; i < 50; i++ {
			items = append(items, map[string]any{"title": "标题" + itoa(int64(start+i+1)), "www_url": "https://e.com/" + itoa(int64(start+i+1)), "extra_info": map[string]any{"hot_value": "1.2万"}})
		}
		ok(map[string]any{"snapshot_id": "snap", "is_ranking": true, "list": items, "has_more": page == "1"})
	case "/v1/item/search/options":
		ok(map[string]any{"timezone": "Asia/Shanghai", "tabs": []map[string]any{{"key": "weibo", "name": "微博"}}})
	case "/v1/item/search":
		q := r.URL.Query()
		if (q.Get("time_range") != "" && q.Get("time_range") != "today") || len(q["tab_keys"]) > 0 {
			if !(authed && f.vip) {
				fail(400, 1018, "user not vip")
				return
			}
		}
		ok(map[string]any{"page": 1, "size": 20, "total": 1, "has_more": false, "list": []map[string]any{{"item_key": "k1", "title": "大模型发布", "tab_key": "weibo", "www_url": "https://weibo.com/1", "last_seen_at": 1763000000}}})
	case "/v1/user/account":
		if !authed {
			fail(401, 1004, "unauthorized")
			return
		}
		end := int64(0)
		if f.vip {
			end = time.Now().Add(24 * time.Hour).Unix()
		}
		ok(map[string]any{"id": 7, "email": "u@example.com", "email_bound": true, "vip_end_at": end})
	case "/v1/nodes/following":
		if !authed {
			fail(401, 1004, "unauthorized")
			return
		}
		ok(map[string]any{"nodes": []map[string]any{{"node_id": "n1", "name": "知乎热榜"}}})
	case "/v1/tabs/following":
		if !authed {
			fail(401, 1004, "unauthorized")
			return
		}
		ok(map[string]any{"tab_info": []map[string]any{}})
	case "/v1/cli/auth/device/code":
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["device_id"] == "" {
			fail(400, 1001, "params")
			return
		}
		ok(map[string]any{"device_code": "dc", "user_code": "WXYZ-2345", "verification_uri": "https://rebang.today/cli/authorize", "expires_in": 600, "interval": 5})
	case "/v1/cli/auth/device/token":
		f.polls++
		if f.polls < 2 {
			fail(400, 1100, "authorization_pending")
			return
		}
		ok(map[string]any{"access_token": f.token, "token_type": "Bearer"})
	case "/v1/cli/auth/logout":
		if !authed {
			fail(401, 1004, "unauthorized")
			return
		}
		ok(nil)
	default:
		fail(404, 1003, "not found")
	}
}

type harness struct {
	api    *fakeAPI
	srv    *httptest.Server
	opened []string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	t.Setenv("REBANG_CONFIG_DIR", t.TempDir())
	t.Setenv("REBANG_HOME", t.TempDir())
	t.Setenv(credentials.EnvToken, "")
	t.Setenv("REBANG_API_BASE", "")
	t.Setenv("REBANG_MANIFEST_URL", "")
	h := &harness{api: &fakeAPI{t: t, token: "rb_cli_valid"}}
	h.srv = httptest.NewServer(h.api)
	t.Cleanup(h.srv.Close)
	t.Setenv("REBANG_API_BASE", h.srv.URL)
	return h
}

// run 执行命令，返回标准输出、标准错误与退出码。tty 模拟终端。
func (h *harness) run(tty bool, args ...string) (string, string, int) {
	var out, errb bytes.Buffer
	a := &App{
		Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb,
		StdoutTTY: tty, Config: config.Default(),
		OpenURL: func(u string) error { h.opened = append(h.opened, u); return nil },
		Now:     time.Now,
		Sleep:   func(context.Context, time.Duration) error { return nil },
	}
	code := Execute(a, args)
	return out.String(), errb.String(), code
}

func decode(t *testing.T, s string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), v); err != nil {
		t.Fatalf("invalid json %v: %s", err, s)
	}
}

func TestDefaultFormatDependsOnTTY(t *testing.T) {
	h := newHarness(t)
	out, _, code := h.run(false, "menus")
	if code != 0 {
		t.Fatal(code)
	}
	var v struct{ Menus []struct{ ID int64 } }
	decode(t, out, &v)
	if len(v.Menus) != 2 {
		t.Fatal(out)
	}
	out, _, _ = h.run(true, "menus")
	if !strings.Contains(out, "综合") || strings.HasPrefix(out, "{") {
		t.Fatal(out)
	}
	out, _, _ = h.run(true, "menus", "-o", "md")
	if !strings.HasPrefix(out, "| ID |") {
		t.Fatal(out)
	}
}

func TestUserAgentAndInvocation(t *testing.T) {
	h := newHarness(t)
	h.run(false, "menus")
	r := h.api.requests[0]
	if r.Header.Get("User-Agent") != buildinfo.UserAgent() || r.Header.Get("X-Rebang-Invocation") != "pipe" {
		t.Fatalf("%v", r.Header)
	}
}

func TestTabsResolvesMenuKey(t *testing.T) {
	h := newHarness(t)
	out, stderr, code := h.run(false, "tabs", "--menu", "tech")
	if code != 0 {
		t.Fatal(stderr)
	}
	var v struct {
		Tabs []struct {
			Key   string
			Child []struct{ Key string }
		}
	}
	decode(t, out, &v)
	if v.Tabs[0].Key != "github" || v.Tabs[0].Child[0].Key != "daily" {
		t.Fatal(out)
	}
	_, stderr, code = h.run(false, "tabs", "--menu", "nope")
	if code != errs.ExitUsage {
		t.Fatalf("code %d %s", code, stderr)
	}
}

func TestNodes(t *testing.T) {
	h := newHarness(t)
	out, _, code := h.run(false, "nodes", "--menu", "1")
	if code != 0 || !strings.Contains(out, "n-weibo") {
		t.Fatal(out)
	}
	out, _, code = h.run(false, "nodes", "--tab", "github")
	if code != 0 || !strings.Contains(out, `"sub_tab": "daily"`) {
		t.Fatal(out)
	}
	_, _, code = h.run(false, "nodes")
	if code != errs.ExitUsage {
		t.Fatal(code)
	}
}

func TestListPaginatesWithSnapshot(t *testing.T) {
	h := newHarness(t)
	out, stderr, code := h.run(false, "list", "--node", "n-weibo", "-n", "60")
	if code != 0 {
		t.Fatal(stderr)
	}
	var v ListResult
	decode(t, out, &v)
	if len(v.Items) != 60 || v.Items[59].Rank != 60 || v.Items[59].Title != "标题60" || v.Items[0].Heat != "1.2万" {
		t.Fatalf("%d %+v", len(v.Items), v.Items[len(v.Items)-1])
	}
	out, _, _ = h.run(true, "list", "--tab", "weibo", "-n", "3", "-o", "md")
	if !strings.HasPrefix(out, "1. [标题1](https://e.com/1)（1.2万）") {
		t.Fatal(out)
	}
}

func TestListLockedNeedsMembership(t *testing.T) {
	h := newHarness(t)
	_, stderr, code := h.run(false, "list", "--node", "locked")
	if code != errs.ExitMembership || !strings.Contains(stderr, `"code": "membership_required"`) {
		t.Fatalf("%d %s", code, stderr)
	}
}

func TestSearchMembership(t *testing.T) {
	h := newHarness(t)
	out, _, code := h.run(false, "search", "大模型")
	if code != 0 {
		t.Fatal(code)
	}
	var v SearchResult
	decode(t, out, &v)
	if v.Items[0].SourceName != "微博" || v.Items[0].URL != "https://weibo.com/1" {
		t.Fatal(out)
	}
	_, stderr, code := h.run(false, "search", "大模型", "--days", "7")
	if code != errs.ExitMembership {
		t.Fatalf("%d %s", code, stderr)
	}
	var e struct{ Error errs.Error }
	decode(t, stderr, &e)
	if e.Error.Code != errs.CodeMembershipRequired || e.Error.Hint == "" {
		t.Fatal(stderr)
	}
	// 会员可以搜索
	h.api.vip = true
	t.Setenv(credentials.EnvToken, "rb_cli_valid")
	_, _, code = h.run(false, "search", "大模型", "--days", "7", "--source", "weibo,zhihu")
	if code != 0 {
		t.Fatal(code)
	}
	last := h.api.requests[len(h.api.requests)-1].URL.Query()
	if last.Get("time_range") != "7d" || len(last["tab_keys"]) != 2 {
		t.Fatal(last)
	}
}

func TestSearchWindow(t *testing.T) {
	now := time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC) // 北京时间 09:00
	tr, s, e, err := SearchWindow(3, now)
	if err != nil || tr != "custom" || s != "2026-11-11" || e != "2026-11-13" {
		t.Fatal(tr, s, e, err)
	}
	if tr, _, _, _ := SearchWindow(30, now); tr != "30d" {
		t.Fatal(tr)
	}
	if _, _, _, err := SearchWindow(400, now); errs.ExitCode(err) != errs.ExitUsage {
		t.Fatal(err)
	}
}

func TestAccountRequiresLogin(t *testing.T) {
	h := newHarness(t)
	_, stderr, code := h.run(false, "account")
	if code != errs.ExitAuth || !strings.Contains(stderr, "unauthorized") {
		t.Fatalf("%d %s", code, stderr)
	}
	// 被吊销的 Token：服务端 401
	t.Setenv(credentials.EnvToken, "rb_cli_revoked")
	_, _, code = h.run(false, "account")
	if code != errs.ExitAuth {
		t.Fatal(code)
	}
	t.Setenv(credentials.EnvToken, "rb_cli_valid")
	out, _, code := h.run(false, "account")
	var v AccountView
	decode(t, out, &v)
	if code != 0 || v.Email != "u@example.com" || v.IsMember || !strings.HasSuffix(v.ManageURL, "/pricing") {
		t.Fatal(out)
	}
}

func TestNetworkErrorKeepsCredentials(t *testing.T) {
	h := newHarness(t)
	if err := credentials.SetToken("rb_cli_valid"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REBANG_API_BASE", "http://127.0.0.1:1")
	_, _, code := h.run(false, "account")
	if code != errs.ExitNetwork {
		t.Fatal(code)
	}
	if tok, _ := credentials.Token(); tok != "rb_cli_valid" {
		t.Fatal("token must be kept on network error")
	}
}

func TestLoginStatusLogout(t *testing.T) {
	h := newHarness(t)
	out, stderr, code := h.run(false, "auth", "login", "--no-browser")
	if code != 0 {
		t.Fatalf("%d %s", code, stderr)
	}
	if !strings.Contains(stderr, "WXYZ-2345") || !strings.Contains(stderr, "user_code=WXYZ-2345") || len(h.opened) != 0 {
		t.Fatalf("stderr %s opened %v", stderr, h.opened)
	}
	if !strings.Contains(out, `"logged_in": true`) {
		t.Fatal(out)
	}
	if tok, src := credentials.Token(); tok != "rb_cli_valid" || src != credentials.SourceFile {
		t.Fatal(tok, src)
	}
	var body map[string]string
	for _, r := range h.api.requests {
		if r.URL.Path == "/v1/cli/auth/device/code" {
			body = map[string]string{"ua": r.UserAgent()}
		}
	}
	if body == nil {
		t.Fatal("device code not requested")
	}

	out, _, code = h.run(false, "auth", "status")
	var st AuthStatus
	decode(t, out, &st)
	if code != 0 || !st.LoggedIn || !st.Verified || st.Source != "file" || strings.Contains(out, "rb_cli_valid") {
		t.Fatal(out)
	}

	out, _, code = h.run(false, "auth", "logout")
	if code != 0 || !strings.Contains(out, `"revoked": true`) {
		t.Fatal(out)
	}
	if tok, _ := credentials.Token(); tok != "" {
		t.Fatal("token not cleared")
	}
	_, _, code = h.run(false, "auth", "status")
	if code != errs.ExitAuth {
		t.Fatal(code)
	}
}

func TestLoginOpensBrowser(t *testing.T) {
	h := newHarness(t)
	_, _, code := h.run(false, "auth", "login")
	if code != 0 || len(h.opened) != 1 {
		t.Fatal(code, h.opened)
	}
}

func TestFollowing(t *testing.T) {
	h := newHarness(t)
	t.Setenv(credentials.EnvToken, "rb_cli_valid")
	out, _, code := h.run(false, "following")
	var v FollowingResult
	decode(t, out, &v)
	if code != 0 || v.Nodes[0].NodeID != "n1" || v.Tabs == nil {
		t.Fatal(out)
	}
}

func TestOpen(t *testing.T) {
	h := newHarness(t)
	out, _, code := h.run(false, "open", "2", "--node", "n-weibo", "--print")
	if code != 0 || !strings.Contains(out, "https://e.com/2") || len(h.opened) != 0 {
		t.Fatal(out)
	}
	_, _, code = h.run(false, "open", "https://example.com/x")
	if code != 0 || h.opened[0] != "https://example.com/x" {
		t.Fatal(h.opened)
	}
	_, _, code = h.run(false, "open", "abc")
	if code != errs.ExitUsage {
		t.Fatal(code)
	}
}

func TestUsageErrors(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{{"nope"}, {"menus", "--bogus"}, {"search"}, {"list"}, {"menus", "-o", "xml"}, {"list", "--node", "a", "--tab", "b"}} {
		_, stderr, code := h.run(false, args...)
		if code != errs.ExitUsage {
			t.Errorf("%v: code %d %s", args, code, stderr)
		}
		var e struct{ Error errs.Error }
		decode(t, stderr, &e)
		if e.Error.Code != errs.CodeInvalidArgument {
			t.Errorf("%v: %s", args, stderr)
		}
	}
	_, stderr, _ := h.run(true, "nope")
	if !strings.HasPrefix(stderr, "错误：") {
		t.Fatal(stderr)
	}
}

func TestVersion(t *testing.T) {
	h := newHarness(t)
	out, _, code := h.run(false, "version")
	var v VersionInfo
	decode(t, out, &v)
	if code != 0 || v.Version != buildinfo.Version || v.Platform == "" {
		t.Fatal(out)
	}
}

func TestNoArgsWithoutTTYPrintsHelp(t *testing.T) {
	h := newHarness(t)
	out, _, code := h.run(false)
	if code != 0 || !strings.Contains(out, "search") {
		t.Fatal(out)
	}
}
