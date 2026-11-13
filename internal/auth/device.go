// Package auth 实现浏览器设备授权登录（RFC 8628，D-10）。
package auth

import (
	"context"
	"net/url"
	"time"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/errs"
)

// Flow 描述一次设备授权。
type Flow struct {
	Client *api.Client
	Sleep  func(context.Context, time.Duration) error
	Now    func() time.Time
	// OnCode 在拿到授权码后回调，用于展示授权码并打开浏览器。
	OnCode func(code *api.DeviceCode, verifyURL string)
}

// VerificationURL 返回带授权码的确认页地址。
func VerificationURL(dc *api.DeviceCode) string {
	if dc.VerificationURIComplete != "" {
		return dc.VerificationURIComplete
	}
	u, err := url.Parse(dc.VerificationURI)
	if err != nil || dc.VerificationURI == "" {
		return dc.VerificationURI
	}
	q := u.Query()
	q.Set("user_code", dc.UserCode)
	u.RawQuery = q.Encode()
	return u.String()
}

// Run 发起授权并轮询，直到拿到 Token、被拒绝或过期。
func (f *Flow) Run(ctx context.Context, req api.DeviceCodeRequest) (*api.DeviceToken, error) {
	dc, err := f.Client.RequestDeviceCode(ctx, req)
	if err != nil {
		return nil, api.MapError(err)
	}
	if dc.DeviceCode == "" || dc.UserCode == "" {
		return nil, errs.New(errs.ExitGeneral, errs.CodeServer, "服务端返回的授权信息不完整", "稍后重试，或升级 rebang")
	}
	if f.OnCode != nil {
		f.OnCode(dc, VerificationURL(dc))
	}
	interval := time.Duration(dc.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	expiresIn := time.Duration(dc.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = 10 * time.Minute
	}
	deadline := f.Now().Add(expiresIn)
	for {
		if err := f.Sleep(ctx, interval); err != nil {
			return nil, err
		}
		if f.Now().After(deadline) {
			return nil, expired()
		}
		tok, err := f.Client.PollDeviceToken(ctx, dc.DeviceCode)
		if err == nil {
			if tok.AccessToken == "" {
				return nil, errs.New(errs.ExitGeneral, errs.CodeServer, "服务端未返回 Token", "稍后重试")
			}
			return tok, nil
		}
		switch api.OAuthErrorOf(err) {
		case api.OAuthPending:
			continue
		case api.OAuthSlowDown:
			interval += 5 * time.Second
			continue
		case api.OAuthAccessDenied:
			return nil, errs.New(errs.ExitAuth, "access_denied", "授权已被拒绝", "如非本人操作请忽略；需要登录时重新运行 rebang auth login")
		case api.OAuthExpired:
			return nil, expired()
		}
		var e *errs.Error
		mapped := api.MapError(err)
		if ee, ok := mapped.(*errs.Error); ok {
			e = ee
		}
		// 网络抖动时继续轮询，直到过期。
		if e != nil && e.Exit == errs.ExitNetwork {
			continue
		}
		return nil, mapped
	}
}

func expired() error {
	return errs.New(errs.ExitAuth, "expired_token", "授权码已过期", "重新运行 rebang auth login")
}
