package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/auth"
	"github.com/rebang-today/rebang-cli/internal/credentials"
	"github.com/rebang-today/rebang-cli/internal/ui"
)

// 本文件是登录相关命令给人看的输出（table 格式与标准错误上的提示），
// JSON 与 md 输出不经过这里。

// formatDate 把 RFC3339 时间格式化为北京时间的日期。
func formatDate(rfc string) string {
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return ""
	}
	return t.In(cst).Format("2006-01-02")
}

// tildePath 把用户目录缩写为 ~，只用于显示。
func tildePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return p
	}
	return "~" + p[len(home):]
}

func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n") + "\n"
}

// loginPrompt 是等待确认时显示在标准错误上的授权码与链接。
// fancy 为 true（标准错误是终端）时突出显示授权码。
func loginPrompt(t ui.Theme, fancy bool, code, url string, opened, tried bool) string {
	var b strings.Builder
	if !fancy {
		fmt.Fprintf(&b, "\n请在浏览器中确认登录，授权码：%s\n%s\n\n", code, url)
		if tried && !opened {
			b.WriteString("无法自动打开浏览器，请手动打开上面的链接。\n")
		}
		return b.String()
	}
	b.WriteString("\n")
	b.WriteString(indent(t.Strong.Render("在浏览器中确认登录"), 2))
	b.WriteString("\n")
	b.WriteString(indent(t.CodeBox(code), 2))
	b.WriteString(indent(t.Dim.Render("请核对网页上显示的授权码与此一致"), 2))
	b.WriteString("\n")
	b.WriteString(indent(t.Label.Render("链接")+"  "+t.Accent.Underline(t.Color).Render(url), 2))
	switch {
	case tried && opened:
		b.WriteString(indent(t.Dim.Render("      已在浏览器中打开；没有看到页面时请手动复制链接"), 2))
	case tried:
		b.WriteString(indent(t.Warning("无法自动打开浏览器，请手动打开上面的链接"), 8))
	default:
		b.WriteString(indent(t.Dim.Render("      在任意设备的浏览器中打开即可"), 2))
	}
	b.WriteString("\n")
	return b.String()
}

// waitingText 是转圈动画后面的文字：剩余时间与取消方式。
func waitingText(t ui.Theme, remaining time.Duration) string {
	left := "剩余 " + ui.Clock(remaining)
	if remaining < time.Minute {
		left = t.Warn.Render(left)
	} else {
		left = t.Dim.Render(left)
	}
	return "等待网页确认… " + left + t.Dim.Render(" · Ctrl+C 取消")
}

func slowDownText(interval time.Duration) string {
	return fmt.Sprintf("服务端请求降低检查频率，已调整为每 %d 秒检查一次，无需操作。", int(interval/time.Second))
}

// tokenExpiry 返回 Token 到期时间的描述。
func tokenExpiry(tok *api.DeviceToken, now time.Time) string {
	var at time.Time
	switch {
	case tok.ExpiresAt > 0:
		at = time.Unix(tok.ExpiresAt, 0)
	case tok.ExpiresIn > 0:
		at = now.Add(time.Duration(tok.ExpiresIn) * time.Second)
	default:
		return ""
	}
	days := int(at.Sub(now).Hours() / 24)
	s := at.In(cst).Format("2006-01-02") + " 到期"
	if days > 0 {
		s += fmt.Sprintf("（%d 天后）", days)
	}
	return s
}

func memberField(v AccountView) ui.Field {
	f := ui.Field{Key: "会员", Value: v.memberText(), Tone: ui.ToneDim}
	if v.IsMember {
		f.Value = ui.MarkOn + " 会员 · " + formatDate(v.MemberExpiresAt) + " 到期"
		f.Tone = ui.ToneOK
	} else if f.Value != "" {
		f.Value = ui.MarkOff + " " + f.Value
	}
	return f
}

type loginInfo struct {
	view     AccountView
	device   string
	expiry   string
	credPath string
}

func loginPretty(li loginInfo) func(ui.Theme, int) string {
	return func(t ui.Theme, width int) string {
		var b strings.Builder
		b.WriteString(t.Success("登录成功"))
		if li.view.Email != "" {
			b.WriteString(t.Dim.Render("，欢迎 ") + li.view.Email)
		}
		b.WriteString("\n\n")
		fields := []ui.Field{
			{Key: "账号", Value: li.view.Email, Tone: ui.ToneStrong},
		}
		if li.view.ID != 0 {
			fields = append(fields, memberField(li.view))
		}
		expiry := li.expiry
		if expiry == "" {
			expiry = "未指定到期时间，可在网站中随时吊销"
		}
		fields = append(fields,
			ui.Field{Key: "设备", Value: li.device},
			ui.Field{Key: "登录有效期", Value: expiry},
		)
		b.WriteString(t.Card(t.KV(fields, 0), width))
		if li.view.ID == 0 {
			b.WriteString(t.Warning("暂未获取到账号信息，稍后可运行 ") + t.Accent.Render("rebang account") + " 查看\n")
		}
		if li.credPath != "" {
			b.WriteString(t.Dim.Render("凭证已保存到 "+tildePath(li.credPath)) + "\n")
		}
		b.WriteString(t.Hint("下一步：运行 ", "rebang") + t.Dim.Render(" 进入交互界面，或 ") + t.Accent.Render("rebang menus") + t.Dim.Render(" 查看全部榜单") + "\n")
		return b.String()
	}
}

func sourceText(st AuthStatus) string {
	switch credentials.Source(st.Source) {
	case credentials.SourceEnv:
		return "环境变量 " + credentials.EnvToken
	case credentials.SourceFile:
		if p, err := credentials.Path(); err == nil {
			return "本机凭证 " + tildePath(p)
		}
		return "本机凭证"
	}
	return ""
}

func statusPretty(st AuthStatus) func(ui.Theme, int) string {
	return func(t ui.Theme, width int) string {
		var b strings.Builder
		switch {
		case st.LoggedIn && st.Verified:
			b.WriteString(t.OK.Render(ui.MarkOn+" 已登录") + "  " + t.Strong.Render(st.Account.Email) + "\n")
		case st.LoggedIn:
			b.WriteString(t.Warn.Render(ui.MarkOn+" 已登录") + t.Dim.Render("（未向服务端验证）") + "\n")
		case st.Token != "":
			b.WriteString(t.Err.Render(ui.MarkOff+" 登录已失效") + "\n")
		default:
			b.WriteString(t.Dim.Render(ui.MarkOff+" 未登录") + "\n")
			return b.String()
		}
		fields := []ui.Field{}
		if st.Account != nil {
			fields = append(fields, ui.Field{Key: "账号", Value: st.Account.Email, Tone: ui.ToneStrong}, memberField(*st.Account))
		}
		fields = append(fields,
			ui.Field{Key: "来源", Value: sourceText(st)},
			ui.Field{Key: "Token", Value: st.Token, Tone: ui.ToneDim},
		)
		b.WriteString("\n")
		b.WriteString(t.Card(t.KV(fields, 0), width))
		return b.String()
	}
}

func accountPretty(v AccountView) func(ui.Theme, int) string {
	return func(t ui.Theme, width int) string {
		bound := "已绑定"
		if !v.EmailBound {
			bound = "未绑定"
		}
		fields := []ui.Field{
			{Key: "账号", Value: v.Email, Tone: ui.ToneStrong},
			{Key: "ID", Value: itoa(v.ID), Tone: ui.ToneDim},
			{Key: "邮箱", Value: bound},
			{Key: "注册时间", Value: formatShort(v.CreatedAt)},
			memberField(v),
		}
		var b strings.Builder
		b.WriteString(t.Card(t.KV(fields, 0), width))
		label := "开通会员："
		if v.IsMember {
			label = "续费会员："
		}
		b.WriteString(t.Dim.Render(label) + t.Accent.Render(v.ManageURL) + "\n")
		return b.String()
	}
}

func logoutPretty(loggedIn, revoked bool) func(ui.Theme, int) string {
	return func(t ui.Theme, _ int) string {
		if !loggedIn {
			return t.Dim.Render(ui.MarkOff+" 当前未登录，无需退出") + "\n"
		}
		s := t.Success("已退出登录")
		if revoked {
			s += t.Dim.Render("，本设备的 Token 已在服务端吊销")
		}
		return s + "\n"
	}
}

// loginWaiter 负责等待确认期间标准错误上的动画与提示。
type loginWaiter struct {
	a        *App
	t        ui.Theme
	spinner  *ui.Spinner
	deadline time.Time
}

func newLoginWaiter(a *App) *loginWaiter { return &loginWaiter{a: a, t: a.ErrTheme()} }

func (w *loginWaiter) start(dc *api.DeviceCode) {
	w.deadline = w.a.Now().Add(auth.Lifetime(dc))
	if !w.a.StderrTTY {
		w.a.Info("等待确认中…（Ctrl+C 取消）\n")
		return
	}
	w.spinner = ui.NewSpinner(w.a.Stderr, w.t, func() string {
		return waitingText(w.t, w.deadline.Sub(w.a.Now()))
	})
	w.spinner.Start()
}

func (w *loginWaiter) slowDown(interval time.Duration) {
	msg := slowDownText(interval)
	if w.spinner != nil {
		w.spinner.Println("  " + w.t.Warning(msg))
		return
	}
	w.a.Info("%s\n", msg)
}

func (w *loginWaiter) stop() {
	if w.spinner != nil {
		w.spinner.Stop()
	}
}
