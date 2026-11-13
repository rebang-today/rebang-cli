// Package release 生成 CDN 上的 cli/manifest.json（D-51、D-55）。
package release

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rebang-today/rebang-cli/internal/update"
)

// archiveRe 匹配 goreleaser 生成的安装包名：rebang_<版本>_<系统>_<架构>.<扩展名>。
var archiveRe = regexp.MustCompile(`^rebang_(.+)_(darwin|linux|windows)_(amd64|arm64)\.(tar\.gz|zip)$`)

// Params 是生成 manifest 的输入。
type Params struct {
	Version    string // 不带 v 前缀
	MinVersion string
	// BaseURL 是 CDN 上 cli 目录的地址，安装包位于 <BaseURL>/v<Version>/。
	BaseURL  string
	NotesURL string
	Now      time.Time
}

// Build 读取 checksums.txt（格式：<sha256>  <文件名>），生成 manifest。
func Build(p Params, checksums io.Reader) (*update.Manifest, error) {
	if _, ok := update.ParseVersion(p.Version); !ok {
		return nil, fmt.Errorf("版本号无效: %q", p.Version)
	}
	if p.MinVersion != "" {
		if _, ok := update.ParseVersion(p.MinVersion); !ok {
			return nil, fmt.Errorf("min_version 无效: %q", p.MinVersion)
		}
		if update.Compare(p.MinVersion, p.Version) > 0 {
			return nil, fmt.Errorf("min_version %s 高于发布版本 %s", p.MinVersion, p.Version)
		}
	}
	base := strings.TrimRight(p.BaseURL, "/")
	if !strings.HasPrefix(base, "https://") {
		return nil, fmt.Errorf("BaseURL 必须是 https 地址: %q", p.BaseURL)
	}
	m := &update.Manifest{
		Version:    p.Version,
		MinVersion: p.MinVersion,
		ReleasedAt: p.Now.UTC().Format(time.RFC3339),
		NotesURL:   p.NotesURL,
		Platforms:  map[string]update.Asset{},
	}
	sc := bufio.NewScanner(checksums)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			continue
		}
		sum, name := fields[0], strings.TrimPrefix(fields[1], "*")
		g := archiveRe.FindStringSubmatch(name)
		if g == nil {
			continue
		}
		if g[1] != p.Version {
			return nil, fmt.Errorf("%s 的版本与 %s 不一致", name, p.Version)
		}
		if len(sum) != 64 {
			return nil, fmt.Errorf("%s 的 sha256 无效", name)
		}
		url := base + "/v" + p.Version + "/" + name
		m.Platforms[g[2]+"/"+g[3]] = update.Asset{URL: url, SHA256: strings.ToLower(sum), SignatureURL: url + ".minisig"}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(m.Platforms) == 0 {
		return nil, fmt.Errorf("checksums 中没有安装包")
	}
	return m, nil
}

// Platforms 返回排序后的平台列表，便于日志输出。
func Platforms(m *update.Manifest) []string {
	out := make([]string, 0, len(m.Platforms))
	for k := range m.Platforms {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
