package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rebang-today/rebang-cli/internal/credentials"
	"github.com/rebang-today/rebang-cli/internal/errs"
)

const summaryKey = "0123456789abcdef0123456789abcdef"

func TestSummaryText(t *testing.T) {
	h := newHarness(t)
	t.Setenv(credentials.EnvToken, "rb_cli_valid")
	out, _, code := h.run(true, "summary", summaryKey)
	if code != 0 {
		t.Fatal(code, out)
	}
	order := []string{"AI 总结", "一句话总结", "要点", "• 要点一", "网友怎么看", "多数支持", "• 网友甲：不错", "“原话”"}
	pos := 0
	for _, s := range order {
		i := strings.Index(out[pos:], s)
		if i < 0 {
			t.Fatalf("缺少或顺序错误 %q\n%s", s, out)
		}
		pos += i + len(s)
	}
	for _, bad := range []string{"**", "旧", "keywords", "debug"} {
		if strings.Contains(out, bad) {
			t.Fatalf("不应出现 %q\n%s", bad, out)
		}
	}
	md, _, _ := h.run(true, "summary", summaryKey, "-o", "md")
	if !strings.Contains(md, "## AI 总结") || !strings.Contains(md, "### 要点") || !strings.Contains(md, "- 要点**一**") || !strings.Contains(md, "> “原话”") {
		t.Fatal(md)
	}
}

func TestSummaryJSON(t *testing.T) {
	h := newHarness(t)
	t.Setenv(credentials.EnvToken, "rb_cli_valid")
	out, _, code := h.run(false, "summary", summaryKey)
	if code != 0 {
		t.Fatal(code)
	}
	var v map[string]json.RawMessage
	decode(t, out, &v)
	if len(v) != 2 || string(v["summary_short"]) != `"一句**话**总结"` {
		t.Fatalf("应只输出 summary_short 与 detail，且保留原文：%s", out)
	}
	var d map[string]any
	decode(t, string(v["detail"]), &d)
	if _, ok := d["points"]; !ok {
		t.Fatal(out)
	}
	if r := h.api.requests[len(h.api.requests)-1]; r.URL.Path != "/v1/item/insight" || r.URL.Query().Get("item_key") != summaryKey {
		t.Fatal(r.URL)
	}
}

func TestSummaryExitCodes(t *testing.T) {
	h := newHarness(t)
	other := strings.Repeat("f", 32)

	// 未登录：不发请求，退出码 3。
	_, stderr, code := h.run(false, "summary", summaryKey)
	if code != errs.ExitAuth || len(h.api.requests) != 0 {
		t.Fatal(code, stderr)
	}

	// 参数错误。
	if _, _, code := h.run(false, "summary", "short"); code != errs.ExitUsage {
		t.Fatal(code)
	}
	if _, _, code := h.run(false, "summary"); code != errs.ExitUsage {
		t.Fatal(code)
	}

	// 暂无总结：单独的退出码与错误码。
	t.Setenv(credentials.EnvToken, "rb_cli_valid")
	_, stderr, code = h.run(false, "summary", other)
	if code != errs.ExitNoSummary {
		t.Fatal(code, stderr)
	}
	var e struct{ Error errs.Error }
	decode(t, stderr, &e)
	if e.Error.Code != errs.CodeSummaryNotFound {
		t.Fatal(stderr)
	}

	// Token 失效：服务端 401，退出码 3。
	t.Setenv(credentials.EnvToken, "rb_cli_bad")
	if _, _, code := h.run(false, "summary", summaryKey); code != errs.ExitAuth {
		t.Fatal(code)
	}
}
