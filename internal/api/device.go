package api

import (
	"context"
	"net/http"
)

// 设备授权接口（RFC 8628），路径与字段以服务端 docs/cli-auth.md 为准。

// DeviceCodeRequest POST /v1/cli/auth/device/code 入参。
type DeviceCodeRequest struct {
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
	Platform      string `json:"platform"`
	ClientVersion string `json:"client_version"`
}

// DeviceCode 发起授权的返回。
type DeviceCode struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// DeviceToken 轮询成功的返回。
type DeviceToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	ExpiresAt   int64  `json:"expires_at"`
}

// RFC 8628 轮询错误码。
const (
	OAuthPending      = "authorization_pending"
	OAuthSlowDown     = "slow_down"
	OAuthAccessDenied = "access_denied"
	OAuthExpired      = "expired_token"
)

// RequestDeviceCode 发起设备授权。
func (c *Client) RequestDeviceCode(ctx context.Context, req DeviceCodeRequest) (*DeviceCode, error) {
	var out DeviceCode
	err := c.do(ctx, http.MethodPost, "/cli/auth/device/code", nil, req, &out)
	return &out, err
}

// PollDeviceToken 轮询一次。返回的错误若是 *APIError，可用 OAuthErrorOf 取得 RFC 8628 错误码。
func (c *Client) PollDeviceToken(ctx context.Context, deviceCode string) (*DeviceToken, error) {
	var out DeviceToken
	err := c.do(ctx, http.MethodPost, "/cli/auth/device/token", nil, map[string]string{"device_code": deviceCode}, &out)
	return &out, err
}

// Logout 吊销当前 Token。
func (c *Client) Logout(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/cli/auth/logout", nil, struct{}{}, nil)
}

// OAuthErrorOf 从错误中提取 RFC 8628 错误码。兼容三种写法：
// 顶层 error 字段、data.error 字段、或 msg 直接为错误码。
func OAuthErrorOf(err error) string {
	a, ok := err.(*APIError)
	if !ok {
		return ""
	}
	if a.OAuthError != "" {
		return a.OAuthError
	}
	switch a.Msg {
	case OAuthPending, OAuthSlowDown, OAuthAccessDenied, OAuthExpired:
		return a.Msg
	}
	return ""
}
