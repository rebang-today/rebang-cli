// Package api 是 rebang 服务端 /v1 接口的客户端。请求与响应结构在本仓库自行定义（D-03）。
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rebang-today/rebang-cli/internal/buildinfo"
	"github.com/rebang-today/rebang-cli/internal/errs"
)

// 服务端业务码（来自服务端统一响应 {code, msg, data}）。
const (
	codeSuccess      = 200
	codeNotModified  = 304
	codeParams       = 1001
	codeNotFound     = 1003
	codeUnauthorized = 1004
	codeForbidden    = 1005
	codeTooMany      = 1014
	codeUserBanned   = 1015
	codeUserNotVip   = 1018
	codeIPBanned     = 1020
	codePause        = 1023
	codeSearchDown   = 1032

	// CLI 设备授权与 Token 相关业务码，见服务端 docs/cli-auth.md。
	codeDevicePending   = 1061
	codeDeviceSlowDown  = 1062
	codeDeviceDenied    = 1063
	codeDeviceExpired   = 1064
	codeCLITokenExpired = 1066
	codeCLITokenRevoked = 1067
	codeCLITokenDenied  = 1068
)

// Invocation 标记调用方式（D-33），供服务端区分人与 Agent 流量。
type Invocation string

const (
	// InvocationTUI 交互界面。
	InvocationTUI Invocation = "tui"
	// InvocationTerminal 在终端中直接执行子命令。
	InvocationTerminal Invocation = "terminal"
	// InvocationPipe 输出被管道或 Agent 读取。
	InvocationPipe Invocation = "pipe"
)

// HeaderInvocation 调用方式请求头。
const HeaderInvocation = "X-Rebang-Invocation"

// Client 是线程安全的 API 客户端。
type Client struct {
	BaseURL    string
	Token      string
	Invocation Invocation
	HTTP       *http.Client
	UserAgent  string
}

// New 创建客户端。
func New(baseURL, token string, inv Invocation) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		Invocation: inv,
		HTTP:       &http.Client{Timeout: 20 * time.Second},
		UserAgent:  buildinfo.UserAgent(),
	}
}

type envelope struct {
	Code  int             `json:"code"`
	Msg   string          `json:"msg"`
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

// APIError 是服务端返回的业务错误，保留原始信息便于诊断。
type APIError struct {
	Status int
	Code   int
	Msg    string
	// OAuthError 为设备授权轮询返回的 RFC 8628 错误码，例如 authorization_pending。
	OAuthError string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("HTTP %d, code %d: %s", e.Status, e.Code, e.Msg)
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	u := c.BaseURL + "/v1" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return errs.New(errs.ExitGeneral, errs.CodeGeneral, "无效的接口地址: "+c.BaseURL, "检查配置项 api_base 或环境变量 REBANG_API_BASE")
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")
	if c.Invocation != "" {
		req.Header.Set(HeaderInvocation, string(c.Invocation))
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		return errs.Network(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return errs.Network(err)
	}
	var env envelope
	jsonErr := json.Unmarshal(raw, &env)
	if resp.StatusCode == http.StatusOK && jsonErr == nil && (env.Code == codeSuccess || env.Code == codeNotModified) {
		if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return errs.New(errs.ExitGeneral, errs.CodeServer, "无法解析服务端响应", "可能需要升级 rebang，运行 rebang update").WithCause(err)
			}
		}
		return nil
	}
	apiErr := &APIError{Status: resp.StatusCode, Code: env.Code, Msg: env.Msg, OAuthError: env.Error}
	if jsonErr != nil {
		apiErr.Msg = http.StatusText(resp.StatusCode)
	}
	if apiErr.OAuthError == "" && len(env.Data) > 0 {
		var d struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(env.Data, &d) == nil {
			apiErr.OAuthError = d.Error
		}
	}
	return apiErr
}

// MapError 把服务端错误映射为 CLI 结构化错误与退出码（D-32、D-34）。
// 只有服务端明确判定凭证无效（401 / 未授权业务码）才返回退出码 3；网络错误与 5xx 不会触发重新登录。
func MapError(err error) error {
	var a *APIError
	if !errors.As(err, &a) {
		return err
	}
	msg := a.Msg
	switch {
	case a.Code == codeCLITokenExpired:
		return errs.Unauthorized("登录已过期").WithCause(a)
	case a.Code == codeCLITokenRevoked:
		return errs.Unauthorized("该设备的登录已被吊销").WithCause(a)
	case a.Status == http.StatusUnauthorized || a.Code == codeUnauthorized:
		return errs.Unauthorized("未登录或登录已失效（" + msg + "）").WithCause(a)
	case a.Code == codeUserNotVip:
		return errs.Membership("该功能需要会员").WithCause(a)
	case a.Status == http.StatusTooManyRequests || a.Code == codeTooMany:
		return errs.New(errs.ExitRateLimited, errs.CodeRateLimited, "请求过于频繁", "稍后再试").WithCause(a)
	case a.Code == codeCLITokenDenied:
		return errs.New(errs.ExitGeneral, errs.CodeForbidden, "命令行登录无法使用该功能（"+msg+"）", "请在网站上操作").WithCause(a)
	case a.Code == codeForbidden:
		return errs.New(errs.ExitGeneral, errs.CodeForbidden, "账号已被停用（"+msg+"）", "如有疑问请在网站反馈").WithCause(a)
	case a.Code == codeUserBanned || a.Code == codeIPBanned:
		return errs.New(errs.ExitGeneral, errs.CodeForbidden, "账号或网络已被限制访问（"+msg+"）", "如有疑问请在网站反馈").WithCause(a)
	case a.Code == codeParams:
		return errs.New(errs.ExitUsage, errs.CodeInvalidArgument, "参数错误（"+msg+"）", "检查 ID 是否正确，运行 rebang <命令> --help 查看用法").WithCause(a)
	case a.Code == codeNotFound || a.Status == http.StatusNotFound:
		return errs.New(errs.ExitGeneral, errs.CodeNotFound, "未找到（"+msg+"）", "").WithCause(a)
	case a.Code == codePause:
		return errs.New(errs.ExitNetwork, errs.CodeServer, "网站升级维护中", "稍后再试").WithCause(a)
	case a.Code == codeSearchDown:
		return errs.New(errs.ExitNetwork, errs.CodeServer, "搜索服务暂不可用", "稍后再试").WithCause(a)
	case a.Status >= 500 || a.Status == 0:
		return errs.New(errs.ExitNetwork, errs.CodeServer, fmt.Sprintf("服务端错误（HTTP %d）", a.Status), "稍后再试，登录状态不受影响").WithCause(a)
	default:
		return errs.New(errs.ExitGeneral, errs.CodeGeneral, fmt.Sprintf("请求失败（%s）", msg), "").WithCause(a)
	}
}
