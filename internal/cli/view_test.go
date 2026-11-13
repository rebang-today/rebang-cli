package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/golden"
	"github.com/rebang-today/rebang-cli/internal/ui"
)

var sampleAccount = AccountView{
	ID: 7, Email: "u@example.com", EmailBound: true, IsMember: true,
	MemberExpiresAt: "2026-12-31T23:59:59+08:00", CreatedAt: "2025-03-01T10:00:00+08:00",
	ManageURL: "https://rebang.today/pricing",
}

func TestLoginViewsGolden(t *testing.T) {
	th := ui.Plain()
	u := "https://rebang.today/cli/authorize?user_code=WXYZ-2345"
	var b strings.Builder
	b.WriteString("## 等待确认（终端）\n")
	b.WriteString(loginPrompt(th, true, "WXYZ-2345", u, true, true))
	b.WriteString(ui.NewSpinner(nil, th, func() string { return waitingText(th, 9*time.Minute+58*time.Second) }).Frame(0) + "\n")
	b.WriteString("  " + th.Warning(slowDownText(10*time.Second)) + "\n")
	b.WriteString("## 等待确认（无法打开浏览器）\n")
	b.WriteString(loginPrompt(th, true, "WXYZ-2345", u, false, true))
	b.WriteString("## 等待确认（非终端）\n")
	b.WriteString(loginPrompt(th, false, "WXYZ-2345", u, false, true))
	b.WriteString("## 登录成功\n")
	b.WriteString(loginPretty(loginInfo{view: sampleAccount, device: "MacBook-Pro", expiry: "2027-02-11 到期（90 天后）", credPath: "/home/me/.config/rebang/auth.json"})(th, 80))
	b.WriteString("## 登录成功（未获取到账号）\n")
	b.WriteString(loginPretty(loginInfo{device: "devbox"})(th, 80))
	golden.Assert(t, "login", b.String())
}

func TestAuthCardsGolden(t *testing.T) {
	th := ui.Plain()
	free := sampleAccount
	free.IsMember = false
	var b strings.Builder
	b.WriteString("## status 已登录\n")
	b.WriteString(statusPretty(AuthStatus{LoggedIn: true, Verified: true, Source: "env", Token: "rb_cli_…3f9a", Account: &sampleAccount})(th, 80))
	b.WriteString("## status --offline\n")
	b.WriteString(statusPretty(AuthStatus{LoggedIn: true, Source: "env", Token: "rb_cli_…3f9a"})(th, 80))
	b.WriteString("## status 已失效\n")
	b.WriteString(statusPretty(AuthStatus{Source: "env", Token: "rb_cli_…3f9a"})(th, 80))
	b.WriteString("## status 未登录\n")
	b.WriteString(statusPretty(AuthStatus{Source: "none"})(th, 80))
	b.WriteString("## account 非会员\n")
	b.WriteString(accountPretty(free)(th, 80))
	b.WriteString("## logout\n")
	b.WriteString(logoutPretty(true, true)(th, 80))
	b.WriteString(logoutPretty(true, false)(th, 80))
	b.WriteString(logoutPretty(false, false)(th, 80))
	golden.Assert(t, "auth_cards", b.String())
}

func TestTokenExpiry(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, cst)
	if s := tokenExpiry(&api.DeviceToken{ExpiresIn: 90 * 86400}, now); s != "2027-02-11 到期（90 天后）" {
		t.Fatal(s)
	}
	if s := tokenExpiry(&api.DeviceToken{ExpiresAt: now.Add(48 * time.Hour).Unix()}, now); s != "2026-11-15 到期（2 天后）" {
		t.Fatal(s)
	}
	if tokenExpiry(&api.DeviceToken{}, now) != "" {
		t.Fatal("未返回到期时间时为空")
	}
}

// 人看的 table 输出快照：列表、搜索、帮助。
func TestHumanTablesGolden(t *testing.T) {
	h := newHarness(t)
	var b strings.Builder
	for _, args := range [][]string{
		{"menus"},
		{"list", "--node", "x", "-n", "3"},
		{"search", "大模型"},
		{"--help"},
		{"list", "--help"},
	} {
		out, errOut, code := h.run(true, args...)
		b.WriteString("$ rebang " + strings.Join(args, " ") + "\n" + out + errOut)
		if code != 0 {
			t.Fatalf("%v exit %d: %s", args, code, errOut)
		}
	}
	golden.Assert(t, "human_tables", b.String())
}

func TestLoginTableAndColor(t *testing.T) {
	h := newHarness(t)
	out, stderr, code := h.run(true, "auth", "login", "--no-browser")
	if code != 0 || !strings.Contains(out, "✓ 登录成功，欢迎 u@example.com") || !strings.Contains(out, "╭") {
		t.Fatalf("%d\n%s\n%s", code, out, stderr)
	}
	if strings.Contains(out+stderr, "\x1b[") {
		t.Fatal("未开启颜色时不应输出控制序列")
	}
	// 未登录时 auth status 的 table 输出与错误。
	h.run(true, "auth", "logout")
	out, stderr, code = h.run(true, "auth", "status")
	if code != 3 || out != "○ 未登录\n" || !strings.HasPrefix(stderr, "错误：尚未登录") {
		t.Fatalf("%d %q %q", code, out, stderr)
	}
}
