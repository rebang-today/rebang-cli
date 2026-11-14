package cli

import (
	"context"
	"os"
	"time"

	"github.com/rebang-today/rebang-cli/internal/buildinfo"
	"github.com/rebang-today/rebang-cli/internal/skill"
	"github.com/rebang-today/rebang-cli/internal/update"
)

// EnvNoNotice 设为 1 时 JSON 结果不附加版本提醒。
const EnvNoNotice = "REBANG_NO_NOTICE"

// noticeCheckTimeout 是每天一次的版本检查的最长等待时间，超时静默。
const noticeCheckTimeout = 1500 * time.Millisecond

// Notice 是 JSON 结果中 "_notice" 的内容（D-63）：提醒 Agent 在完成任务后告诉用户。
type Notice struct {
	Update *UpdateNotice `json:"update,omitempty"`
	Skill  *SkillNotice  `json:"skill,omitempty"`
}

// UpdateNotice 表示有新版本。
type UpdateNotice struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	Command string `json:"command"`
}

// SkillNotice 表示本机已安装的 Agent Skill 与程序版本不一致。
type SkillNotice struct {
	Installed string   `json:"installed"`
	Expected  string   `json:"expected"`
	Dirs      []string `json:"dirs"`
	Command   string   `json:"command"`
}

type noticeCache struct{ n *Notice }

// Notice 返回本次调用的版本提醒，没有时返回 nil。新版本信息来自每天最多一次的检查缓存
// （state.json，与交互界面共用）；缓存过期时联网检查一次，失败或超时静默。
func (a *App) Notice() *Notice {
	if a.notice != nil {
		return a.notice.n
	}
	a.notice = &noticeCache{}
	if a.noNotice || os.Getenv(EnvNoNotice) == "1" || update.IsDev(buildinfo.Version) {
		return nil
	}
	n := &Notice{}
	ctx, cancel := context.WithTimeout(context.Background(), noticeCheckTimeout)
	defer cancel()
	if m, err := update.CheckDaily(ctx, a.Config.Manifest(), buildinfo.Version, a.Now()); err == nil && m != nil {
		n.Update = &UpdateNotice{Current: buildinfo.Version, Latest: m.Version, Command: "rebang update"}
	}
	if home, err := homeDir(); err == nil {
		for _, ag := range skill.KnownAgents(home) {
			st := skill.StatusOf(ag, buildinfo.Version)
			if !st.Installed || st.UpToDate {
				continue
			}
			if n.Skill == nil {
				n.Skill = &SkillNotice{Installed: st.Version, Expected: buildinfo.Version, Command: "rebang skill install"}
			}
			n.Skill.Dirs = append(n.Skill.Dirs, st.Dir)
		}
	}
	if n.Update == nil && n.Skill == nil {
		return nil
	}
	a.notice.n = n
	return n
}
