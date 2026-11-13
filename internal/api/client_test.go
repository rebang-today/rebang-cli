package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebang-today/rebang-cli/internal/errs"
)

func ok(w http.ResponseWriter, data any) {
	json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "success", "data": data})
}

func fail(w http.ResponseWriter, status, code int, msg string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"code": code, "msg": msg})
}

func TestHeadersAndMenus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if !strings.HasPrefix(ua, "rebang-cli/") || strings.Contains(strings.ToLower(ua), "go-http-client") {
			t.Errorf("bad UA %q", ua)
		}
		if r.Header.Get(HeaderInvocation) != "pipe" {
			t.Errorf("invocation %q", r.Header.Get(HeaderInvocation))
		}
		if r.Header.Get("Authorization") != "Bearer rb_cli_t" {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/v1/menus" {
			t.Errorf("path %s", r.URL.Path)
		}
		ok(w, map[string]any{"menus": []map[string]any{{"id": 1, "menu_name": "综合", "menu_key": "all", "menu_path": "/"}}})
	}))
	defer srv.Close()
	c := New(srv.URL+"/", "rb_cli_t", InvocationPipe)
	menus, err := c.Menus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(menus) != 1 || menus[0].Name != "综合" || menus[0].Key != "all" {
		t.Fatalf("%+v", menus)
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		status, code int
		exit         int
	}{
		{401, 1004, errs.ExitAuth},
		{400, 1018, errs.ExitMembership},
		{403, 1018, errs.ExitMembership},
		{429, 1014, errs.ExitRateLimited},
		{400, 1001, errs.ExitUsage},
		{500, 1002, errs.ExitNetwork},
		{503, 1007, errs.ExitNetwork},
		{400, 9999, errs.ExitGeneral},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fail(w, tc.status, tc.code, "x")
		}))
		_, err := New(srv.URL, "", InvocationPipe).Account(context.Background())
		srv.Close()
		if got := errs.ExitCode(MapError(err)); got != tc.exit {
			t.Errorf("status %d code %d: exit %d want %d", tc.status, tc.code, got, tc.exit)
		}
	}
}

func TestNetworkErrorIsNotAuth(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	_, err := New(url, "rb_cli_x", InvocationPipe).Menus(context.Background())
	if errs.ExitCode(MapError(err)) != errs.ExitNetwork {
		t.Fatalf("got %v", err)
	}
	// 非 JSON 的 502 也只算服务端错误
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		w.Write([]byte("<html>bad gateway</html>"))
	}))
	defer srv2.Close()
	_, err = New(srv2.URL, "rb_cli_x", InvocationPipe).Menus(context.Background())
	if errs.ExitCode(MapError(err)) != errs.ExitNetwork {
		t.Fatalf("got %v", err)
	}
}

func TestSearchQueryAndList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/item/search":
			q := r.URL.Query()
			if q.Get("q") != "AI" || q.Get("time_range") != "7d" || len(q["tab_keys"]) != 2 || q.Get("size") != "20" {
				t.Errorf("query %v", q)
			}
			ok(w, map[string]any{"page": 1, "size": 20, "total": 1, "list": []map[string]any{{"item_key": "k", "title": "t", "www_url": "https://e.com"}}})
		case "/v1/latest_list":
			q := r.URL.Query()
			if q.Get("node_id") != "n1" || q.Get("page") != "1" {
				t.Errorf("query %v", q)
			}
			ok(w, map[string]any{"snapshot_id": "s", "list": []map[string]any{{"title": "a", "www_url": "u", "extra_info": map[string]any{"hot": "1万"}}}, "content_access": map[string]any{"mode": "preview", "reason": "membership_required", "preview_limit": 10}})
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "", InvocationPipe)
	p, err := c.Search(context.Background(), SearchQuery{Query: "AI", TimeRange: "7d", TabKeys: []string{"a", "b"}, Size: 20})
	if err != nil || len(p.List) != 1 || p.List[0].URL != "https://e.com" {
		t.Fatalf("%+v %v", p, err)
	}
	lp, err := c.LatestList(context.Background(), ListQuery{NodeID: "n1"})
	if err != nil || len(lp.List) != 1 || lp.ContentAccess.Mode != "preview" {
		t.Fatalf("%+v %v", lp, err)
	}
}

func TestOAuthErrorExtraction(t *testing.T) {
	for _, body := range []string{
		`{"code":1101,"msg":"authorization_pending"}`,
		`{"code":1101,"msg":"pending","data":{"error":"authorization_pending"}}`,
		`{"error":"authorization_pending"}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(400)
			w.Write([]byte(body))
		}))
		_, err := New(srv.URL, "", InvocationPipe).PollDeviceToken(context.Background(), "dc")
		srv.Close()
		if OAuthErrorOf(err) != OAuthPending {
			t.Errorf("body %s: got %q (%v)", body, OAuthErrorOf(err), err)
		}
	}
}

// TestDeviceErrorCodes 使用服务端真实的返回格式：HTTP 400 + {code, msg}，msg 为中文。
func TestDeviceErrorCodes(t *testing.T) {
	cases := map[int]string{1061: OAuthPending, 1062: OAuthSlowDown, 1063: OAuthAccessDenied, 1064: OAuthExpired}
	for code, want := range cases {
		code := code
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(400)
			fmt.Fprintf(w, `{"code":%d,"msg":"等待在网页上确认授权。"}`, code)
		}))
		_, err := New(srv.URL, "", InvocationPipe).PollDeviceToken(context.Background(), "dc")
		srv.Close()
		if got := OAuthErrorOf(err); got != want {
			t.Errorf("code %d: got %q, want %q (%v)", code, got, want, err)
		}
	}
	if OAuthErrorOf(&APIError{Status: 400, Code: 1001}) != "" {
		t.Error("其他业务码不应识别为 OAuth 错误")
	}
}

func TestCLITokenErrors(t *testing.T) {
	cases := []struct {
		status, code, exit int
	}{
		{401, 1066, errs.ExitAuth},
		{401, 1067, errs.ExitAuth},
		{403, 1068, errs.ExitGeneral},
		{403, 1005, errs.ExitGeneral},
		{429, 1014, errs.ExitRateLimited},
		{503, 1000, errs.ExitNetwork},
	}
	for _, c := range cases {
		err := MapError(&APIError{Status: c.status, Code: c.code, Msg: "x"})
		if got := errs.ExitCode(err); got != c.exit {
			t.Errorf("%d/%d: exit %d, want %d (%v)", c.status, c.code, got, c.exit, err)
		}
	}
}

func TestCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New("http://127.0.0.1:1", "", InvocationPipe).Menus(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}
