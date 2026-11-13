package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/rebang-today/rebang-cli/internal/fsutil"
)

// 默认地址均为公开地址，可在配置文件或环境变量中覆盖，便于本地联调。
const (
	DefaultAPIBase         = "https://api.rebang.today"
	DefaultWebBase         = "https://rebang.today"
	DefaultManifestURL     = "https://rebang.today/cli/manifest.json"
	DefaultRefreshInterval = 5 * time.Minute
	FileName               = "config.toml"
)

// Config 对应 config.toml。
type Config struct {
	// Output 默认输出格式：json、md、table；留空时按是否为终端自动选择。
	Output string `toml:"output"`
	// AutoUpdate 开启后交互使用时发现新版本自动升级。
	AutoUpdate bool `toml:"auto_update"`
	// RefreshInterval 交互界面定时刷新间隔，例如 "5m"；"0" 关闭。
	RefreshInterval string `toml:"refresh_interval"`
	// Theme 交互界面主题：auto、dark、light。
	Theme string `toml:"theme"`
	// APIBase 接口地址，留空使用默认值。
	APIBase string `toml:"api_base,omitempty"`
	// WebBase 网站地址，留空使用默认值。
	WebBase string `toml:"web_base,omitempty"`
	// ManifestURL 版本信息文件地址，留空使用默认值。
	ManifestURL string `toml:"manifest_url,omitempty"`
}

// Default 返回默认配置。
func Default() Config {
	return Config{RefreshInterval: DefaultRefreshInterval.String(), Theme: "auto"}
}

// Load 读取配置文件；文件不存在时返回默认配置。
func Load() (Config, error) {
	cfg := Default()
	p, err := Path(FileName)
	if err != nil {
		return cfg, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if _, err := toml.Decode(string(b), &cfg); err != nil {
		return Default(), fmt.Errorf("解析配置文件 %s 失败: %w", p, err)
	}
	return cfg, cfg.Validate()
}

// Save 原子写入配置文件。
func Save(cfg Config) error {
	p, err := Path(FileName)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString("# rebang 配置文件，说明见 https://github.com/rebang-today/rebang-cli\n")
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(p, buf.Bytes(), 0o600)
}

// Validate 校验配置取值。
func (c Config) Validate() error {
	switch c.Output {
	case "", "json", "md", "table":
	default:
		return fmt.Errorf("配置项 output 取值无效: %q（可选 json、md、table）", c.Output)
	}
	switch c.Theme {
	case "", "auto", "dark", "light":
	default:
		return fmt.Errorf("配置项 theme 取值无效: %q（可选 auto、dark、light）", c.Theme)
	}
	if _, err := c.Refresh(); err != nil {
		return err
	}
	return nil
}

// Refresh 返回定时刷新间隔，0 表示关闭。
func (c Config) Refresh() (time.Duration, error) {
	s := strings.TrimSpace(c.RefreshInterval)
	if s == "" {
		return DefaultRefreshInterval, nil
	}
	if s == "0" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("配置项 refresh_interval 取值无效: %q（例如 5m、30s、0）", s)
	}
	if d > 0 && d < 30*time.Second {
		d = 30 * time.Second
	}
	return d, nil
}

// API 返回接口地址：环境变量 REBANG_API_BASE > 配置文件 > 默认值。
func (c Config) API() string {
	return pick(os.Getenv("REBANG_API_BASE"), c.APIBase, DefaultAPIBase)
}

// Web 返回网站地址：环境变量 REBANG_WEB_BASE > 配置文件 > 默认值。
func (c Config) Web() string {
	return pick(os.Getenv("REBANG_WEB_BASE"), c.WebBase, DefaultWebBase)
}

// Manifest 返回版本信息文件地址：环境变量 REBANG_MANIFEST_URL > 配置文件 > 默认值。
func (c Config) Manifest() string {
	return pick(os.Getenv("REBANG_MANIFEST_URL"), c.ManifestURL, DefaultManifestURL)
}

func pick(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	return ""
}
