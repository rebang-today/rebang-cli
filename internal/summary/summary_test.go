package summary

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"github.com/rebang-today/rebang-cli/internal/api"
)

const sample = `{
  "quotes": ["旧版金句，应忽略"],
  "reactions": {"mood": "多数人**支持**", "voices": [{"by": "网友甲", "text": "说得对", "quote": "原话一句"}]},
  "viewpoints": [{"stance": "赞成", "text": "理由**很充分**", "by": "知乎用户A", "quote": "我觉得可以"}],
  "timeline": [{"time": "", "text": "事情开始"}, {"time": "9月1日", "text": "**正式发布**"}],
  "points": ["第一条**要点**", "第二条"]
}`

func detail(t *testing.T, s string) api.InsightDetail {
	t.Helper()
	return (&api.Insight{Detail: json.RawMessage(s)}).ParseDetail()
}

func TestSectionsOrderAndIgnoreUnknown(t *testing.T) {
	text := strings.Join(Plain(Sections(detail(t, sample)), 80), "\n")
	order := []string{"要点", "• 第一条要点", "经过", "• 事情开始", "• 9月1日 正式发布", "观点", "• 赞成", "  理由很充分", "  “我觉得可以”", "  —— 知乎用户A", "网友怎么看", "多数人支持", "• 网友甲：说得对", "  “原话一句”"}
	pos := 0
	for _, s := range order {
		i := strings.Index(text[pos:], s)
		if i < 0 {
			t.Fatalf("缺少或顺序错误 %q\n%s", s, text)
		}
		pos += i + len(s)
	}
	if strings.Contains(text, "金句") || strings.Contains(text, "**") {
		t.Fatalf("应忽略 quotes 并去掉加粗标记\n%s", text)
	}
}

func TestMissingSections(t *testing.T) {
	if got := Sections(detail(t, `null`)); got != nil {
		t.Fatalf("%+v", got)
	}
	text := strings.Join(Plain(Sections(detail(t, `{"viewpoints":[{"stance":"反对","text":"","by":"","quote":""}]}`)), 40), "\n")
	if text != "观点\n• 反对" {
		t.Fatalf("%q", text)
	}
}

func TestParseBold(t *testing.T) {
	cases := map[string][]Span{
		"a**b**c":     {{Text: "a"}, {Text: "b", Bold: true}, {Text: "c"}},
		"**全部**":      {{Text: "全部", Bold: true}},
		"没有标记":        {{Text: "没有标记"}},
		"不成对**x":      {{Text: "不成对**x"}},
		"空****内容":     {{Text: "空****内容"}},
		"*斜体* 与 `代码`": {{Text: "*斜体* 与 `代码`"}},
	}
	for in, want := range cases {
		got := ParseBold(in)
		if len(got) != len(want) {
			t.Fatalf("%q → %+v", in, got)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%q → %+v", in, got)
			}
		}
	}
}

func TestRenderStyles(t *testing.T) {
	st := Style{
		Bold:    func(s string) string { return "<b>" + s + "</b>" },
		Quote:   func(s string) string { return "<q>" + s + "</q>" },
		Label:   func(s string) string { return "<l>" + s + "</l>" },
		Heading: func(s string) string { return "<h>" + s + "</h>" },
		Dim:     func(s string) string { return "<d>" + s + "</d>" },
	}
	text := strings.Join(Render(Sections(detail(t, sample)), 80, st), "\n")
	for _, s := range []string{"<h>要点</h>", "第一条<b>要点</b>", "<l>9月1日</l> <b>正式发布</b>", "<q>“我觉得可以”</q>", "<d>—— 知乎用户A</d>", "<l>网友甲：</l>说得对"} {
		if !strings.Contains(text, s) {
			t.Fatalf("缺少 %q\n%s", s, text)
		}
	}
}

func TestWrapByDisplayWidth(t *testing.T) {
	long := strings.Repeat("中文内容", 10) + " hello world " + strings.Repeat("x", 30)
	lines := []Line{{Kind: Bullet, Text: long}, {Kind: Quote, Indent: 2, Text: long}, {Kind: Para, Text: "前**加粗跨行的一段文字**后"}}
	for _, w := range []int{1, 2, 3, 5, 9, 17, 30} {
		out := Plain(lines, w)
		if len(out) == 0 {
			t.Fatalf("%d: 无输出", w)
		}
		for _, l := range out {
			if runewidth.StringWidth(l) > w {
				t.Fatalf("宽度 %d 溢出：%q", w, l)
			}
		}
	}
	out := Plain([]Line{{Kind: Bullet, Text: "一二三四五六七八"}}, 10)
	if strings.Join(out, "|") != "• 一二三四|  五六七八" {
		t.Fatalf("悬挂缩进：%q", out)
	}
	out = Plain([]Line{{Kind: Para, Text: "abc defgh ij"}}, 8)
	if strings.Join(out, "|") != "abc|defgh ij" {
		t.Fatalf("英文按单词折行：%q", out)
	}
	if Render(lines, 0, Style{}) != nil {
		t.Fatal("宽度 0 应无输出")
	}
}

func TestWrapKinsoku(t *testing.T) {
	for _, w := range []int{4, 6, 8, 10, 12} {
		for _, l := range Plain([]Line{{Kind: Para, Text: "今天天气很好，所谓AI、高中生；结论：完毕。"}}, w) {
			if l != "" && noLineStart(string([]rune(l)[0])) {
				t.Fatalf("宽 %d 时行首为标点：%q", w, l)
			}
			if runewidth.StringWidth(l) > w {
				t.Fatalf("宽 %d 超出：%q", w, l)
			}
		}
	}
}
