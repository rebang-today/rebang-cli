// Package update 实现基于 manifest.json 的升级（D-50 ~ D-52）。
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rebang-today/rebang-cli/internal/buildinfo"
	"github.com/rebang-today/rebang-cli/internal/errs"
)

// Manifest 是 CDN 上 cli/manifest.json 的结构。
type Manifest struct {
	Version    string           `json:"version"`
	MinVersion string           `json:"min_version"`
	ReleasedAt string           `json:"released_at,omitempty"`
	NotesURL   string           `json:"notes_url,omitempty"`
	Platforms  map[string]Asset `json:"platforms"`
}

// Asset 是某个平台的下载信息，键为 <os>/<arch>。
type Asset struct {
	URL          string `json:"url"`
	SHA256       string `json:"sha256"`
	SignatureURL string `json:"signature_url"`
}

// HTTPClient 用于下载，测试时可替换。
var HTTPClient = &http.Client{Timeout: 5 * time.Minute}

func get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, errs.Network(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errs.New(errs.ExitNetwork, errs.CodeNetwork, fmt.Sprintf("下载失败（HTTP %d）: %s", resp.StatusCode, url), "稍后重试")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, errs.Network(err)
	}
	return b, nil
}

// FetchManifest 读取版本信息文件。
func FetchManifest(ctx context.Context, url string) (*Manifest, error) {
	b, err := get(ctx, url, 1<<20)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, errs.New(errs.ExitGeneral, errs.CodeServer, "版本信息文件格式错误", "").WithCause(err)
	}
	if _, ok := ParseVersion(m.Version); !ok {
		return nil, errs.New(errs.ExitGeneral, errs.CodeServer, "版本信息文件缺少有效的 version", "")
	}
	return &m, nil
}
