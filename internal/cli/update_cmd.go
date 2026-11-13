package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/rebang-today/rebang-cli/internal/buildinfo"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/output"
	"github.com/rebang-today/rebang-cli/internal/update"
)

// UpdateResult 是 update 命令的 JSON 输出。
type UpdateResult struct {
	Current       string   `json:"current_version"`
	Latest        string   `json:"latest_version"`
	MinVersion    string   `json:"min_version,omitempty"`
	Updated       bool     `json:"updated"`
	CheckOnly     bool     `json:"check_only,omitempty"`
	NotesURL      string   `json:"notes_url,omitempty"`
	SkillsUpdated []string `json:"skills_updated,omitempty"`
}

// Executable 返回当前程序路径，测试时可替换。
var Executable = func() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func newUpdateCmd(a *App) *cobra.Command {
	var check, force bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "升级到最新版本",
		Long: `升级到最新版本。

从官方 CDN 读取版本信息，下载对应平台的安装包，校验 sha256 与 minisign 签名后替换当前程序，
并同步已安装的 Agent Skill。通过 Homebrew 安装时请使用 brew upgrade rebang。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			m, err := update.FetchManifest(ctx, a.Config.Manifest())
			if err != nil {
				return err
			}
			update.Record(m, a.Now())
			cur := buildinfo.Version
			res := UpdateResult{Current: cur, Latest: m.Version, MinVersion: m.MinVersion, NotesURL: m.NotesURL, CheckOnly: check}
			newer := update.Compare(m.Version, cur) > 0 || (force && !check)
			if !newer || check {
				msg := "已是最新版本"
				if newer {
					msg = "发现新版本 " + m.Version + "，运行 rebang update 升级"
				}
				return a.Writer().Write(updateOutput(res, msg))
			}
			exe, err := Executable()
			if err != nil {
				return errs.New(errs.ExitGeneral, errs.CodeGeneral, "无法定位当前程序: "+err.Error(), "")
			}
			if update.ManagedBy(exe) == "homebrew" {
				return errs.New(errs.ExitGeneral, "managed_install", "当前程序通过 Homebrew 安装", "运行 brew upgrade rebang 升级")
			}
			a.Info("正在下载 %s …\n", m.Version)
			bin, err := update.Download(ctx, m, buildinfo.Platform())
			if err != nil {
				return err
			}
			if err := update.Replace(exe, bin); err != nil {
				var e *errs.Error
				if errors.As(err, &e) {
					return e
				}
				return errs.New(errs.ExitGeneral, errs.CodeGeneral, "替换程序失败: "+err.Error(), "使用有权限的用户重新运行，或重新执行安装脚本")
			}
			res.Updated = true
			res.SkillsUpdated = syncSkillsWith(ctx, exe)
			return a.Writer().Write(updateOutput(res, "已升级到 "+m.Version))
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "只检查是否有新版本，不升级")
	cmd.Flags().BoolVar(&force, "force", false, "即使已是最新版本也重新安装")
	return cmd
}

// syncSkillsWith 用新程序同步 skill，保证 skill 与程序版本一致（D-60）。
func syncSkillsWith(ctx context.Context, exe string) []string {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, exe, "skill", "sync").Output()
	if err != nil {
		return nil
	}
	var v struct {
		Updated []string `json:"updated"`
	}
	if jsonUnmarshal(out, &v) != nil {
		return nil
	}
	return v.Updated
}

func updateOutput(r UpdateResult, msg string) output.Result {
	return output.Result{
		Data:     r,
		Columns:  []output.Column{{Title: "当前版本"}, {Title: "最新版本"}, {Title: "结果"}},
		Rows:     [][]string{{r.Current, r.Latest, msg}},
		Markdown: func() string { return msg },
	}
}

func init() {
	doctorChecks = append(doctorChecks, doctorUpdate)
}

func doctorUpdate(ctx context.Context, a *App) []Check {
	var cs []Check
	add := func(name, status, detail string) { cs = append(cs, Check{name, status, detail}) }
	mctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if m, err := update.FetchManifest(mctx, a.Config.Manifest()); err != nil {
		add("版本检查", "warn", "无法读取版本信息："+errs.From(err).Message)
	} else {
		update.Record(m, a.Now())
		switch {
		case !update.IsDev(buildinfo.Version) && m.MinVersion != "" && update.Compare(buildinfo.Version, m.MinVersion) < 0:
			add("版本检查", "fail", "当前版本低于最低要求 "+m.MinVersion+"，运行 rebang update")
		case update.Compare(m.Version, buildinfo.Version) > 0:
			add("版本检查", "warn", "有新版本 "+m.Version+"，运行 rebang update")
		default:
			add("版本检查", "ok", "已是最新版本")
		}
	}

	return cs
}
