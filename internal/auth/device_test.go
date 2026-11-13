package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/errs"
)

// fakeServer 模拟服务端 docs/cli-auth.md 约定的设备授权接口。
func fakeServer(t *testing.T, script []string) (*httptest.Server, *int32) {
	var polls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/cli/auth/device/code":
			var req api.DeviceCodeRequest
			json.NewDecoder(r.Body).Decode(&req)
			if req.DeviceID == "" || req.Platform == "" || req.ClientVersion == "" {
				t.Errorf("missing fields %+v", req)
			}
			json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{
				"device_code": "dc", "user_code": "ABCD-EFGH", "verification_uri": "https://rebang.today/cli/authorize",
				"expires_in": 600, "interval": 5,
			}})
		case "/v1/cli/auth/device/token":
			n := atomic.AddInt32(&polls, 1)
			step := script[int(n)-1]
			if step == "ok" {
				json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"access_token": "rb_cli_tok", "token_type": "Bearer"}})
				return
			}
			if step == "500" {
				w.WriteHeader(500)
				return
			}
			w.WriteHeader(400)
			codes := map[string]int{"authorization_pending": 1061, "slow_down": 1062, "access_denied": 1063, "expired_token": 1064}
			json.NewEncoder(w).Encode(map[string]any{"code": codes[step], "msg": "等待在网页上确认授权。"})
		}
	}))
	return srv, &polls
}

var slowDowns []time.Duration

func run(t *testing.T, script []string) (*api.DeviceToken, error, []time.Duration, string) {
	slowDowns = nil
	srv, _ := fakeServer(t, script)
	defer srv.Close()
	var sleeps []time.Duration
	now := time.Unix(0, 0)
	var shown string
	f := &Flow{
		Client: api.New(srv.URL, "", api.InvocationTerminal),
		Sleep: func(_ context.Context, d time.Duration) error {
			sleeps = append(sleeps, d)
			now = now.Add(d)
			return nil
		},
		Now:        func() time.Time { return now },
		OnCode:     func(_ *api.DeviceCode, u string) { shown = u },
		OnSlowDown: func(d time.Duration) { slowDowns = append(slowDowns, d) },
	}
	tok, err := f.Run(context.Background(), api.DeviceCodeRequest{DeviceID: "d", DeviceName: "n", Platform: "darwin/arm64", ClientVersion: "0.1.0"})
	return tok, err, sleeps, shown
}

func TestFlowSuccessWithSlowDown(t *testing.T) {
	tok, err, sleeps, shown := run(t, []string{"authorization_pending", "slow_down", "500", "ok"})
	if err != nil || tok.AccessToken != "rb_cli_tok" {
		t.Fatalf("%v %v", tok, err)
	}
	want := []time.Duration{5 * time.Second, 5 * time.Second, 10 * time.Second, 10 * time.Second}
	for i := range want {
		if sleeps[i] != want[i] {
			t.Fatalf("sleeps %v", sleeps)
		}
	}
	if len(slowDowns) != 1 || slowDowns[0] != 10*time.Second {
		t.Fatalf("slow_down 回调 %v", slowDowns)
	}
	if shown != "https://rebang.today/cli/authorize?user_code=ABCD-EFGH" {
		t.Fatalf("url %s", shown)
	}
}

func TestFlowDenied(t *testing.T) {
	_, err, _, _ := run(t, []string{"authorization_pending", "access_denied"})
	if errs.ExitCode(err) != errs.ExitAuth || errs.From(err).Code != "access_denied" {
		t.Fatalf("%v", err)
	}
}

func TestFlowExpired(t *testing.T) {
	_, err, _, _ := run(t, []string{"expired_token"})
	if errs.From(err).Code != "expired_token" {
		t.Fatalf("%v", err)
	}
}

func TestFlowLocalDeadline(t *testing.T) {
	script := make([]string, 200)
	for i := range script {
		script[i] = "authorization_pending"
	}
	_, err, _, _ := run(t, script)
	if errs.From(err).Code != "expired_token" {
		t.Fatalf("%v", err)
	}
}
