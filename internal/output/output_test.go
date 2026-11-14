package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/golden"
	"github.com/rebang-today/rebang-cli/internal/ui"
)

func TestResolve(t *testing.T) {
	cases := []struct {
		flag, cfg string
		tty       bool
		want      Format
	}{
		{"", "", true, Table},
		{"", "", false, JSON},
		{"", "md", false, MD},
		{"json", "md", true, JSON},
		{"markdown", "", true, MD},
	}
	for _, c := range cases {
		got, err := Resolve(c.flag, c.cfg, c.tty)
		if err != nil || got != c.want {
			t.Errorf("Resolve(%q,%q,%v)=%v,%v want %v", c.flag, c.cfg, c.tty, got, err, c.want)
		}
	}
	_, err := Resolve("xml", "", true)
	if errs.ExitCode(err) != errs.ExitUsage {
		t.Fatalf("want usage error, got %v", err)
	}
}

func TestRenderTableCJK(t *testing.T) {
	s := RenderTable([]Column{{Title: "#"}, {Title: "标题"}}, [][]string{{"1", "中文标题"}, {"10", "ab"}}, 0)
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("lines %q", lines)
	}
	if !strings.HasPrefix(lines[2], "1   中文标题") {
		t.Fatalf("alignment: %q", lines[2])
	}
}

func TestRenderTableTruncate(t *testing.T) {
	s := RenderTable([]Column{{Title: "t", Max: 5}}, [][]string{{"abcdefghij"}}, 0)
	if !strings.Contains(s, "abcd…") {
		t.Fatal(s)
	}
	if RenderTable(nil, nil, 0) != "（无数据）\n" {
		t.Fatal("empty")
	}
}

// TestTableStripsInvisible 表格去掉不可见字符；JSON 保留原文。
func TestTableStripsInvisible(t *testing.T) {
	title := "\u200b\u2061\u2062\u2063\u2064\u202c\ufeff\u00ad标题\u200b\u2061\u2062\u2063\u2064\u202c\ufeff\u00ad\n第二行"
	s := RenderTable([]Column{{Title: "#"}, {Title: "标题"}}, [][]string{{"1", title}}, 40)
	if !strings.Contains(s, "1  标题 第二行") {
		t.Fatalf("%q", s)
	}
	for _, r := range s {
		if r >= 0x2000 && r <= 0x206f || r == 0xfeff || r == 0xad {
			t.Fatalf("表格仍含 %U：%q", r, s)
		}
	}
	var b bytes.Buffer
	w := Writer{Out: &b, Format: JSON}
	if err := w.Write(Result{Data: map[string]string{"title": title}}); err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(b.Bytes(), &got); err != nil || got["title"] != title {
		t.Fatalf("JSON 应保留原文：%q", b.String())
	}
}

func TestMarkdown(t *testing.T) {
	s := RenderMarkdownTable([]Column{{Title: "a"}}, [][]string{{"x|y\nz"}})
	if !strings.Contains(s, `| x\|y z |`) {
		t.Fatal(s)
	}
	if MDLink("a]b", "http://x/(1)") != `[a\]b](http://x/(1%29)` {
		t.Fatal(MDLink("a]b", "http://x/(1)"))
	}
}

func TestWriteJSONNoEscape(t *testing.T) {
	var b bytes.Buffer
	WriteJSON(&b, map[string]string{"u": "a&b<c>"})
	if !strings.Contains(b.String(), "a&b<c>") {
		t.Fatal(b.String())
	}
}

func TestWriteError(t *testing.T) {
	var b bytes.Buffer
	WriteError(&b, errs.Membership(""), true)
	var v struct {
		Error struct{ Code, Message, Hint string }
	}
	if err := json.Unmarshal(b.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Error.Code != errs.CodeMembershipRequired || v.Error.Hint == "" {
		t.Fatalf("%+v", v)
	}
	b.Reset()
	WriteError(&b, errors.New("boom"), false)
	if !strings.HasPrefix(b.String(), "错误：boom") {
		t.Fatal(b.String())
	}
}

func TestTableGolden(t *testing.T) {
	cols := []Column{{Title: "#", Kind: KindRank}, {Title: "标题", Max: 20, Shrink: true}, {Title: "热度", Kind: KindAccent}, {Title: "链接", Kind: KindDim}}
	rows := [][]string{
		{"1", "大模型发布会今晚举行", "129万", "https://e.com/1"},
		{"2", "一个非常非常非常非常长的标题会被截断", "86万", "https://e.com/2"},
		{"10", "ab", "", "https://e.com/10"},
	}
	var b bytes.Buffer
	w := Writer{Out: &b, Format: Table, Width: 60}
	if err := w.Write(Result{Columns: cols, Rows: rows, Footer: "\n用浏览器打开第 N 条：rebang open N"}); err != nil {
		t.Fatal(err)
	}
	golden.Assert(t, "table", b.String())

	c := ui.New(true)
	b.Reset()
	w.Theme = &c
	_ = w.Write(Result{Columns: cols, Rows: rows})
	if !strings.Contains(b.String(), "\x1b[") {
		t.Fatal("彩色主题应输出颜色")
	}
	if got := golden.StripANSI(b.String()); got != RenderTable(cols, rows, 60) {
		t.Fatalf("去掉颜色后应与无颜色表格一致\n%s\n%s", got, RenderTable(cols, rows, 60))
	}
}

func TestPrettyOnlyForTable(t *testing.T) {
	r := Result{Data: map[string]int{"a": 1}, Columns: []Column{{Title: "a"}}, Rows: [][]string{{"1"}},
		Pretty: func(ui.Theme, int) string { return "卡片\n" }}
	for f, want := range map[Format]string{Table: "卡片\n", JSON: "{\n  \"a\": 1\n}\n", MD: "| a |\n| --- |\n| 1 |\n"} {
		var b bytes.Buffer
		_ = Writer{Out: &b, Format: f}.Write(r)
		if b.String() != want {
			t.Errorf("%s: %q", f, b.String())
		}
	}
}

func TestWriteErrorTheme(t *testing.T) {
	e := errs.New(errs.ExitNetwork, errs.CodeServer, "搜索服务暂不可用", "稍后再试").WithCause(errors.New("HTTP 503"))
	var plain, themed bytes.Buffer
	WriteError(&plain, e, false)
	WriteErrorTheme(&themed, e, ui.Plain())
	if plain.String() != themed.String() {
		t.Fatalf("无颜色时应与原格式一致：%q %q", plain.String(), themed.String())
	}
	themed.Reset()
	WriteErrorTheme(&themed, e, ui.New(true))
	golden.Assert(t, "error_color", golden.StripANSI(themed.String()))
}

func TestTableThemeColors(t *testing.T) {
	cols := []Column{{Title: "#", Kind: KindRank}, {Title: "热度", Kind: KindAccent}}
	s := RenderTableTheme(ui.New(true), cols, [][]string{{"1", "100万"}, {"4", "10万"}}, 80)
	if !strings.Contains(s, "38;5;202m1") {
		t.Fatalf("前 3 名序号应为橙色：%q", s)
	}
	if !strings.Contains(s, "38;5;33m100万") {
		t.Fatalf("强调色应为蓝色：%q", s)
	}
	if strings.Contains(s, "38;5;166") {
		t.Fatalf("不应再出现旧的橙色强调色：%q", s)
	}
}
