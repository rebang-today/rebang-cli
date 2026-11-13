package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/rebang-today/rebang-cli/internal/errs"
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
	if len(lines) != 3 {
		t.Fatalf("lines %q", lines)
	}
	if !strings.HasPrefix(lines[1], "1   中文标题") {
		t.Fatalf("alignment: %q", lines[1])
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
