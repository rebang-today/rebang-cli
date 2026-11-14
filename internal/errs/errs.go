// Package errs 定义结构化错误与固定退出码（D-32）。
package errs

import (
	"errors"
	"fmt"
)

// 退出码（D-32），1.0 之后视为稳定接口。
const (
	ExitOK          = 0
	ExitGeneral     = 1
	ExitUsage       = 2
	ExitAuth        = 3
	ExitMembership  = 4
	ExitRateLimited = 5
	ExitNetwork     = 6
	ExitUpgrade     = 7
	// ExitNoSummary 条目暂无 AI 总结（rebang summary）。
	ExitNoSummary = 8
)

// 错误码，与退出码一一对应的大类之外允许更细的 code。
const (
	CodeGeneral            = "general_error"
	CodeInvalidArgument    = "invalid_argument"
	CodeUnauthorized       = "unauthorized"
	CodeMembershipRequired = "membership_required"
	CodeRateLimited        = "rate_limited"
	CodeNetwork            = "network_error"
	CodeServer             = "server_error"
	CodeUpgradeRequired    = "upgrade_required"
	CodeNotFound           = "not_found"
	CodeForbidden          = "forbidden"
	CodeSummaryNotFound    = "summary_not_found"
)

// Error 是 CLI 对外输出的结构化错误。
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	Exit    int    `json:"-"`
	Cause   error  `json:"-"`
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

// New 构造错误。
func New(exit int, code, message, hint string) *Error {
	return &Error{Code: code, Message: message, Hint: hint, Exit: exit}
}

// Usage 返回参数错误。
func Usage(format string, args ...any) *Error {
	return New(ExitUsage, CodeInvalidArgument, fmt.Sprintf(format, args...), "运行 rebang <命令> --help 查看用法")
}

// Unauthorized 返回未登录错误。
func Unauthorized(message string) *Error {
	if message == "" {
		message = "未登录或登录已失效"
	}
	return New(ExitAuth, CodeUnauthorized, message, "运行 rebang auth login 登录，或设置 REBANG_TOKEN")
}

// Membership 返回需要会员错误。
func Membership(message string) *Error {
	if message == "" {
		message = "该功能需要会员"
	}
	return New(ExitMembership, CodeMembershipRequired, message, "开通会员请访问 https://rebang.today ，运行 rebang account 查看会员状态")
}

// Network 返回网络错误。
func Network(cause error) *Error {
	e := New(ExitNetwork, CodeNetwork, "网络请求失败", "检查网络连接后重试，运行 rebang doctor 诊断")
	e.Cause = cause
	return e
}

// From 把任意错误转换为结构化错误。
func From(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Code: CodeGeneral, Message: err.Error(), Exit: ExitGeneral}
}

// ExitCode 返回错误对应的退出码。
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	return From(err).Exit
}

// WithCause 附加底层错误。
func (e *Error) WithCause(err error) *Error { e.Cause = err; return e }
