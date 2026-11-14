package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebang-today/rebang-cli/internal/errs"
)

func key32(i int) string { return fmt.Sprintf("%032d", i) }

func TestInsightsBatchChunks(t *testing.T) {
	var sizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/item/insights/batch" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type %q", r.Header.Get("Content-Type"))
		}
		var req struct {
			ItemKeys []string `json:"item_keys"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		sizes = append(sizes, len(req.ItemKeys))
		items := map[string]any{}
		for _, k := range req.ItemKeys {
			// 只有偶数编号有总结。
			var n int
			fmt.Sscanf(k, "%d", &n)
			if n%2 == 0 {
				items[k] = map[string]string{"summary_short": "总结" + k[28:]}
			}
		}
		ok(w, map[string]any{"items": items})
	}))
	defer srv.Close()

	keys := make([]string, 0, 125)
	for i := 0; i < 120; i++ {
		keys = append(keys, key32(i))
	}
	keys = append(keys, key32(1), key32(2), "", key32(3), key32(4)) // 重复与空值不重复发送
	got, err := New(srv.URL, "t", InvocationTUI).InsightsBatch(context.Background(), keys)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(sizes) != "[50 50 20]" {
		t.Fatalf("应按 50 分批：%v", sizes)
	}
	if len(got) != 60 || got[key32(118)].SummaryShort != "总结0118" {
		t.Fatalf("%d %+v", len(got), got[key32(118)])
	}
	if _, ok := got[key32(1)]; ok {
		t.Fatal("没有总结的条目不应出现")
	}

	// 空输入不发请求。
	sizes = nil
	if m, err := New(srv.URL, "t", InvocationTUI).InsightsBatch(context.Background(), nil); err != nil || len(m) != 0 || sizes != nil {
		t.Fatalf("%v %v %v", m, err, sizes)
	}
}

func TestInsightDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/item/insight" || r.URL.Query().Get("item_key") != key32(7) {
			t.Errorf("%s %s", r.URL.Path, r.URL.RawQuery)
		}
		w.Write([]byte(`{"code":200,"msg":"ok","data":{"summary_short":"一句话","detail":{"points":["事实**一**"],"quotes":["旧分区"],"reactions":{"mood":"","voices":[]},"viewpoints":[{"stance":"赞成","text":"理由","by":"","quote":"原话"}]},"keywords":["k"]}}`))
	}))
	defer srv.Close()
	in, err := New(srv.URL, "t", InvocationPipe).Insight(context.Background(), key32(7))
	if err != nil {
		t.Fatal(err)
	}
	d := in.ParseDetail()
	if in.SummaryShort != "一句话" || len(d.Points) != 1 || len(d.Viewpoints) != 1 || d.Viewpoints[0].Quote != "原话" {
		t.Fatalf("%+v %+v", in, d)
	}
	if d.Reactions != nil {
		t.Fatal("空的 reactions 应视为缺失")
	}
	b, _ := json.Marshal(in)
	if strings.Contains(string(b), "keywords") {
		t.Fatalf("不应保留 keywords：%s", b)
	}

	// detail 为 null 或格式异常时不崩溃。
	for _, raw := range []string{`null`, `"x"`, `{"points":"bad","timeline":[{"time":"9点","text":"发生"}]}`} {
		d := (&Insight{Detail: json.RawMessage(raw)}).ParseDetail()
		if raw[0] == '{' && (len(d.Timeline) != 1 || d.Points != nil) {
			t.Fatalf("%s → %+v", raw, d)
		}
	}
}

func TestInsightRouteMissingIsNotNoSummary(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	_, err := New(srv.URL, "t", InvocationPipe).Insight(context.Background(), key32(1))
	if err == nil || IsNotFound(err) {
		t.Fatalf("纯文本 404 不应视为暂无总结：%v", err)
	}
}

func TestInsightErrors(t *testing.T) {
	cases := []struct {
		status, code int
		notFound     bool
		exit         int
	}{
		{404, 1003, true, errs.ExitGeneral},
		{401, 1004, false, errs.ExitAuth},
		{500, 1002, false, errs.ExitNetwork},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fail(w, tc.status, tc.code, "x")
		}))
		c := New(srv.URL, "t", InvocationPipe)
		_, err := c.Insight(context.Background(), key32(1))
		if IsNotFound(err) != tc.notFound {
			t.Errorf("%d: IsNotFound=%v", tc.status, IsNotFound(err))
		}
		if got := errs.ExitCode(MapError(err)); got != tc.exit {
			t.Errorf("%d: exit %d want %d", tc.status, got, tc.exit)
		}
		_, err = c.InsightsBatch(context.Background(), []string{key32(1)})
		if got := errs.ExitCode(MapError(err)); tc.status == 401 && got != errs.ExitAuth {
			t.Errorf("batch 401 应映射为未登录：%d", got)
		}
		srv.Close()
	}
}
