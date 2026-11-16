package cli

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/auth"
	"github.com/rebang-today/rebang-cli/internal/buildinfo"
	"github.com/rebang-today/rebang-cli/internal/credentials"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/output"
)

func newAuthCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "登录、退出、查看登录状态"}
	cmd.AddCommand(newLoginCmd(a), newLogoutCmd(a), newAuthStatusCmd(a))
	return cmd
}

func newLoginCmd(a *App) *cobra.Command {
	var noBrowser bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "通过浏览器授权登录",
		Long: `通过浏览器授权登录（设备授权）。

CLI 展示授权码并打开浏览器，在网站登录并确认后即可完成登录。
网站支持的全部登录方式都可使用。在 SSH 等没有桌面的环境中使用 --no-browser，
在任意设备的浏览器中打开显示的链接即可。

JSON 输出时（非终端或 -o json），拿到授权码后立即在标准错误输出一行 JSON，便于 Agent
转告用户：{"event":"login_pending","verification_uri":…,"verification_uri_complete":…,
"user_code":…,"expires_in":…}。命令在用户确认、拒绝或授权码过期后自行退出。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, src := credentials.Token(); src == credentials.SourceEnv {
				a.Info("注意：已设置环境变量 %s，它的优先级高于本次登录保存的凭证。\n", credentials.EnvToken)
			}
			deviceID, err := credentials.DeviceID()
			if err != nil {
				return errs.New(errs.ExitGeneral, errs.CodeGeneral, "无法写入凭证目录: "+err.Error(), "检查 ~/.config/rebang 的权限")
			}
			host, _ := os.Hostname()
			c := api.New(a.APIBase(), "", a.Invocation())
			wait := newLoginWaiter(a)
			defer wait.stop()
			flow := &auth.Flow{
				Client: c,
				Sleep:  a.Sleep,
				Now:    a.Now,
				OnCode: func(dc *api.DeviceCode, u string) {
					opened := false
					if !noBrowser {
						opened = a.OpenURL(u) == nil
					}
					a.Info("%s", loginPrompt(a.ErrTheme(), a.StderrTTY, dc.UserCode, u, opened, !noBrowser))
					if a.Format() == output.JSON {
						a.loginPendingLine(dc, u)
					}
					wait.start(dc)
				},
				OnSlowDown: wait.slowDown,
			}
			tok, err := flow.Run(cmd.Context(), api.DeviceCodeRequest{
				DeviceID: deviceID, DeviceName: host, Platform: buildinfo.Platform(), ClientVersion: buildinfo.Version,
			})
			wait.stop()
			if err != nil {
				return err
			}
			if err := credentials.SetToken(tok.AccessToken); err != nil {
				return errs.New(errs.ExitGeneral, errs.CodeGeneral, "保存凭证失败: "+err.Error(), "")
			}
			acct, _ := api.New(a.APIBase(), tok.AccessToken, a.Invocation()).Account(cmd.Context())
			view := AccountView{}
			if acct != nil && acct.ID != 0 {
				view = toAccountView(acct, a.Now(), a.Config.Web())
			}
			p, _ := credentials.Path()
			if a.Format() != output.Table {
				a.Info("登录成功。凭证已保存到 %s\n", p)
			}
			return a.Writer().Write(output.Result{
				Data:    map[string]any{"logged_in": true, "account": view},
				Columns: []output.Column{{Title: "账号"}, {Title: "会员"}},
				Rows:    [][]string{{view.Email, view.memberText()}},
				Pretty:  loginPretty(loginInfo{view: view, device: host, expiry: tokenExpiry(tok, a.Now()), credPath: p}),
			})
		},
	}
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "不自动打开浏览器（适用于 SSH 等无桌面环境）")
	return cmd
}

func newLogoutCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "退出登录",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			f, _ := credentials.Load()
			_, src := credentials.Token()
			if f.Token == "" {
				if src == credentials.SourceEnv {
					return errs.New(errs.ExitGeneral, errs.CodeGeneral, "当前凭证来自环境变量 "+credentials.EnvToken+"，无法通过 logout 清除", "取消设置该环境变量即可")
				}
				return a.Writer().Write(output.Result{Data: map[string]any{"logged_in": false, "revoked": false}, Columns: []output.Column{{Title: "状态"}}, Rows: [][]string{{"未登录"}}, Pretty: logoutPretty(false, false)})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			revoked := true
			if err := api.New(a.APIBase(), f.Token, a.Invocation()).Logout(ctx); err != nil {
				mapped := errs.From(api.MapError(err))
				// Token 已失效时视同吊销成功；其他失败只提示，本地凭证照常删除。
				if mapped.Exit != errs.ExitAuth {
					revoked = false
					a.Info("注意：本机已退出，但未能同步到服务器（%s），可在网站设置中移除这台设备。\n", mapped.Message)
				}
			}
			if err := credentials.ClearToken(); err != nil {
				return errs.New(errs.ExitGeneral, errs.CodeGeneral, "删除本地凭证失败: "+err.Error(), "")
			}
			if src == credentials.SourceEnv {
				a.Info("注意：环境变量 %s 仍然有效。\n", credentials.EnvToken)
			}
			return a.Writer().Write(output.Result{
				Data:    map[string]any{"logged_in": false, "revoked": revoked},
				Columns: []output.Column{{Title: "状态"}},
				Rows:    [][]string{{"已退出登录"}},
				Pretty:  logoutPretty(true, revoked),
			})
		},
	}
}

// AuthStatus 是 auth status 的 JSON 输出。
type AuthStatus struct {
	LoggedIn bool         `json:"logged_in"`
	Source   string       `json:"source"`
	Token    string       `json:"token,omitempty"`
	Verified bool         `json:"verified"`
	Account  *AccountView `json:"account,omitempty"`
}

func newAuthStatusCmd(a *App) *cobra.Command {
	var offline bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "查看登录状态（未登录时退出码为 3）",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			tok, src := credentials.Token()
			st := AuthStatus{Source: string(src)}
			if tok == "" {
				_ = a.Writer().Write(statusResult(st))
				return errs.Unauthorized("尚未登录")
			}
			st.LoggedIn = true
			st.Token = credentials.Mask(tok)
			if !offline {
				acct, err := api.New(a.APIBase(), tok, a.Invocation()).Account(cmd.Context())
				if err != nil {
					mapped := errs.From(api.MapError(err))
					if mapped.Exit == errs.ExitAuth {
						st.LoggedIn = false
						_ = a.Writer().Write(statusResult(st))
						return errs.Unauthorized("登录已失效，或这台设备已在网站中被移除")
					}
					_ = a.Writer().Write(statusResult(st))
					return mapped
				}
				st.Verified = true
				v := toAccountView(acct, a.Now(), a.Config.Web())
				st.Account = &v
			}
			return a.Writer().Write(statusResult(st))
		},
	}
	cmd.Flags().BoolVar(&offline, "offline", false, "只检查本地凭证，不访问服务端")
	return cmd
}

func statusResult(st AuthStatus) output.Result {
	state := "未登录"
	if st.LoggedIn {
		state = "已登录"
		if !st.Verified {
			state = "已登录（未验证）"
		}
	}
	email, member := "", ""
	if st.Account != nil {
		email, member = st.Account.Email, st.Account.memberText()
	}
	return output.Result{
		Data:    st,
		Columns: []output.Column{{Title: "状态"}, {Title: "来源"}, {Title: "Token"}, {Title: "账号"}, {Title: "会员"}},
		Rows:    [][]string{{state, string(st.Source), st.Token, email, member}},
		Pretty:  statusPretty(st),
	}
}

// AccountView 是 account 命令的 JSON 输出。会员判断以服务端返回的到期时间为准（D-14）。
type AccountView struct {
	ID              int64  `json:"id"`
	Email           string `json:"email"`
	EmailBound      bool   `json:"email_bound"`
	CreatedAt       string `json:"created_at,omitempty"`
	IsMember        bool   `json:"is_member"`
	MemberExpiresAt string `json:"member_expires_at,omitempty"`
	ManageURL       string `json:"manage_url"`
}

func (v AccountView) memberText() string {
	if v.ID == 0 {
		return ""
	}
	if v.IsMember {
		return "会员，" + formatShort(v.MemberExpiresAt) + " 到期"
	}
	if v.MemberExpiresAt != "" {
		return "非会员（" + formatShort(v.MemberExpiresAt) + " 已到期）"
	}
	return "非会员"
}

func toAccountView(acct *api.Account, now time.Time, web string) AccountView {
	return AccountView{
		ID:              acct.ID,
		Email:           acct.Email,
		EmailBound:      acct.EmailBound,
		CreatedAt:       formatUnix(acct.CreatedAt),
		IsMember:        acct.VipEndAt > now.Unix(),
		MemberExpiresAt: formatUnix(acct.VipEndAt),
		ManageURL:       web + "/pricing",
	}
}

func newAccountCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "account",
		Short: "账号与会员状态",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireLogin(); err != nil {
				return err
			}
			acct, err := a.Client().Account(cmd.Context())
			if err != nil {
				return api.MapError(err)
			}
			if acct.ID == 0 {
				return errs.New(errs.ExitGeneral, errs.CodeServer, "服务端未返回账号信息", "稍后重试")
			}
			v := toAccountView(acct, a.Now(), a.Config.Web())
			return a.Writer().Write(output.Result{
				Data:    v,
				Columns: []output.Column{{Title: "ID"}, {Title: "邮箱"}, {Title: "会员"}, {Title: "开通或续费"}},
				Rows:    [][]string{{itoa(v.ID), v.Email, v.memberText(), v.ManageURL}},
				Pretty:  accountPretty(v),
			})
		},
	}
}

// ToAccountView 把账号信息转换为输出结构，供交互界面复用。
func ToAccountView(acct *api.Account, now time.Time, web string) AccountView {
	return toAccountView(acct, now, web)
}

// MemberText 返回会员状态的中文描述。
func (v AccountView) MemberText() string { return v.memberText() }

// LoginPending 是 JSON 模式下 auth login 拿到授权码后写到标准错误的一行（D-64）。
type LoginPending struct {
	Event                   string `json:"event"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	UserCode                string `json:"user_code"`
	ExpiresIn               int    `json:"expires_in"`
}

func (a *App) loginPendingLine(dc *api.DeviceCode, u string) {
	uri := dc.VerificationURI
	if uri == "" {
		uri = u
	}
	b, err := json.Marshal(LoginPending{Event: "login_pending", VerificationURI: uri, VerificationURIComplete: u, UserCode: dc.UserCode, ExpiresIn: dc.ExpiresIn})
	if err != nil {
		return
	}
	a.Info("%s\n", b)
}
