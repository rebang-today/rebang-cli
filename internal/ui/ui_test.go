package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/rebang-today/rebang-cli/internal/golden"
)

func TestColorEnabled(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "")
	if ColorEnabled(false) {
		t.Fatal("非终端不应上色")
	}
	if !ColorEnabled(true) {
		t.Fatal("终端应上色")
	}
	t.Setenv("NO_COLOR", "")
	if !ColorEnabled(true) {
		t.Fatal("NO_COLOR 为空时按规范视为未设置")
	}
	t.Setenv("NO_COLOR", "1")
	if ColorEnabled(true) {
		t.Fatal("设置 NO_COLOR 时不应上色")
	}
}

func TestColorEnabledDumbTerm(t *testing.T) {
	t.Setenv("TERM", "dumb")
	if ColorEnabled(true) {
		t.Fatal("TERM=dumb 不应上色")
	}
}

func TestPlainHasNoEscapes(t *testing.T) {
	th := Plain()
	s := th.Success("登录成功") + th.Failure("x") + th.Card(th.KV([]Field{{Key: "账号", Value: "a", Tone: ToneOK}}, 0), 0) + th.CodeBox("ABCD-EFGH")
	if strings.Contains(s, "\x1b") {
		t.Fatalf("无颜色主题不应输出控制序列：%q", s)
	}
	if c := New(true); !strings.Contains(c.Success("x"), "\x1b[") {
		t.Fatal("彩色主题应输出颜色")
	}
}

func TestCardGolden(t *testing.T) {
	th := Plain()
	body := th.KV([]Field{
		{Key: "账号", Value: "u@example.com"},
		{Key: "空值会被跳过", Value: ""},
		{Key: "登录有效期", Value: "2027-02-11 到期"},
	}, 0)
	golden.Assert(t, "card", th.Card(body, 80)+th.CodeBox("WXYZ-2345")+"\n")
	// 终端太窄时不画边框。
	if s := th.Card(body, 10); strings.Contains(s, "╭") {
		t.Fatal(s)
	}
}

func TestSpinnerFrameAndClock(t *testing.T) {
	sp := NewSpinner(nil, Plain(), func() string { return "等待" })
	if sp.Frame(0) != "⠋ 等待" || sp.Frame(11) != "⠙ 等待" {
		t.Fatal(sp.Frame(0), sp.Frame(11))
	}
	if Clock(9*time.Minute+58*time.Second) != "9:58" || Clock(-time.Second) != "0:00" {
		t.Fatal(Clock(9*time.Minute + 58*time.Second))
	}
}
