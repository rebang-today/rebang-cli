package update

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/rebang-today/rebang-cli/internal/config"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/fsutil"
)

// State 记录最近一次版本检查结果，保存在配置目录的 state.json。
type State struct {
	LastCheck     time.Time `json:"last_check"`
	LatestVersion string    `json:"latest_version,omitempty"`
	MinVersion    string    `json:"min_version,omitempty"`
}

const stateFile = "state.json"

// LoadState 读取状态，失败时返回空状态。
func LoadState() State {
	var s State
	p, err := config.Path(stateFile)
	if err != nil {
		return s
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(b, &s)
	return s
}

// SaveState 保存状态。
func SaveState(s State) error {
	p, err := config.Path(stateFile)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return fsutil.WriteFileAtomic(p, b, 0o600)
}

// Record 用新拿到的 manifest 更新状态。
func Record(m *Manifest, now time.Time) {
	_ = SaveState(State{LastCheck: now.UTC(), LatestVersion: m.Version, MinVersion: m.MinVersion})
}

// EnforceMinVersion 依据最近一次检查结果，版本低于 min_version 时拒绝执行（退出码 7）。
// 非交互调用不联网检查，只使用本地记录（D-50）。
func EnforceMinVersion(current string) error {
	if IsDev(current) {
		return nil
	}
	s := LoadState()
	if s.MinVersion != "" && Compare(current, s.MinVersion) < 0 {
		return UpgradeRequired(current, s.MinVersion)
	}
	return nil
}

// UpgradeRequired 返回需要升级错误。
func UpgradeRequired(current, min string) error {
	return errs.New(errs.ExitUpgrade, errs.CodeUpgradeRequired,
		"当前版本 "+current+" 已不再受支持，最低要求 "+min, "运行 rebang update 升级")
}

// CheckDaily 每天最多联网检查一次，有新版本时返回 manifest，否则返回 nil。供交互界面使用。
func CheckDaily(ctx context.Context, manifestURL, current string, now time.Time) (*Manifest, error) {
	if IsDev(current) {
		return nil, nil
	}
	s := LoadState()
	if !s.LastCheck.IsZero() && now.Sub(s.LastCheck) < 24*time.Hour {
		if s.LatestVersion != "" && Compare(s.LatestVersion, current) > 0 {
			return &Manifest{Version: s.LatestVersion, MinVersion: s.MinVersion}, nil
		}
		return nil, nil
	}
	m, err := FetchManifest(ctx, manifestURL)
	if err != nil {
		return nil, err
	}
	Record(m, now)
	if Compare(m.Version, current) > 0 {
		return m, nil
	}
	return nil, nil
}
