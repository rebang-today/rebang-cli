package cli

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rebang-today/rebang-cli/internal/buildinfo"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/output"
	"github.com/rebang-today/rebang-cli/internal/skill"
)

func homeDir() (string, error) {
	if h := os.Getenv("REBANG_HOME"); h != "" { // 测试用
		return h, nil
	}
	return os.UserHomeDir()
}

func newSkillCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "管理 Agent Skill",
		Long: `管理随程序发布的 Agent Skill。

rebang 是命令行手册，rebang-* 是基于它的应用（如 rebang-research 话题研究），
作为一组安装到各 Agent 的 skill 目录（如 ~/.claude/skills/rebang），
rebang update 升级后会自动同步已安装的 skill。`,
	}
	var agents []string
	var dir string
	var force bool
	targets := func() ([]skill.Agent, error) {
		if dir != "" {
			return []skill.Agent{{Name: "custom", Root: dir}}, nil
		}
		home, err := homeDir()
		if err != nil {
			return nil, err
		}
		sel, err := skill.Select(home, agents)
		if err != nil {
			return nil, errs.Usage("%s", err.Error())
		}
		return sel, nil
	}
	addTargetFlags := func(c *cobra.Command) {
		c.Flags().StringSliceVar(&agents, "agent", nil, "目标 Agent："+strings.Join(skill.AgentNames(), "、")+"、all（默认本机已安装的 Agent）")
		c.Flags().StringVar(&dir, "dir", "", "自定义 skill 根目录，skill 安装到 <dir>/rebang、<dir>/rebang-*")
	}

	install := &cobra.Command{
		Use:   "install",
		Short: "安装或更新 skill",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ts, err := targets()
			if err != nil {
				return err
			}
			var sts []skill.Status
			for _, t := range ts {
				if err := skill.Install(t, buildinfo.Version, force); err != nil {
					var fe *skill.ForeignError
					if errors.As(err, &fe) {
						return errs.New(errs.ExitGeneral, "skill_conflict", fe.Dir+" 已存在且不是由 rebang 安装的",
							"如果是通过 npx skills 等方式安装的，用原来的方式更新即可；确认可以覆盖后加 --force")
					}
					return errs.New(errs.ExitGeneral, errs.CodeGeneral, "安装 skill 失败: "+err.Error(), "")
				}
				st := skill.StatusOf(t, buildinfo.Version)
				st.Agent = t.Name
				sts = append(sts, st)
			}
			return a.Writer().Write(skillResult(sts))
		},
	}
	addTargetFlags(install)
	install.Flags().BoolVar(&force, "force", false, "覆盖不是由 rebang 安装的同名目录")

	uninstall := &cobra.Command{
		Use:   "uninstall",
		Short: "卸载 skill",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(agents) == 0 && dir == "" {
				agents = []string{"all"}
			}
			ts, err := targets()
			if err != nil {
				return err
			}
			rows := [][]string{}
			type res struct {
				Agent   string `json:"agent"`
				Dir     string `json:"dir"`
				Removed bool   `json:"removed"`
			}
			out := []res{}
			for _, t := range ts {
				removed, err := skill.Uninstall(t)
				if err != nil && !errors.Is(err, skill.ErrForeign) {
					return errs.New(errs.ExitGeneral, errs.CodeGeneral, "卸载失败: "+err.Error(), "")
				}
				out = append(out, res{t.Name, t.Root, removed})
				state := "未安装"
				if removed {
					state = "已卸载"
				} else if err != nil {
					state = "跳过（非 rebang 安装）"
				}
				rows = append(rows, []string{t.Name, t.Root, state})
			}
			return a.Writer().Write(output.Result{
				Data:    map[string]any{"skills": out},
				Columns: []output.Column{{Title: "Agent"}, {Title: "目录"}, {Title: "结果"}},
				Rows:    rows,
			})
		},
	}
	addTargetFlags(uninstall)

	status := &cobra.Command{
		Use:   "status",
		Short: "查看 skill 安装状态",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(agents) == 0 && dir == "" {
				agents = []string{"all"}
			}
			ts, err := targets()
			if err != nil {
				return err
			}
			var sts []skill.Status
			for _, t := range ts {
				sts = append(sts, skill.StatusOf(t, buildinfo.Version))
			}
			return a.Writer().Write(skillResult(sts))
		},
	}
	addTargetFlags(status)

	sync := &cobra.Command{
		Use:    "sync",
		Short:  "把已安装的 skill 同步到当前版本",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, err := homeDir()
			if err != nil {
				return err
			}
			updated, err := skill.Sync(home, buildinfo.Version)
			if updated == nil {
				updated = []string{}
			}
			if err != nil {
				return errs.New(errs.ExitGeneral, errs.CodeGeneral, "同步 skill 失败: "+err.Error(), "")
			}
			return output.WriteJSON(a.Stdout, map[string]any{"version": buildinfo.Version, "updated": updated})
		},
	}
	cmd.AddCommand(install, uninstall, status, sync)
	return cmd
}

func skillResult(sts []skill.Status) output.Result {
	rows := [][]string{}
	for _, s := range sts {
		state := "未安装"
		switch {
		case s.Foreign:
			state = "存在同名目录（非 rebang 安装）"
		case s.Installed && s.UpToDate:
			state = "已安装 " + s.Version + "：" + strings.Join(s.Skills, "、")
		case s.Installed:
			state = "已安装 " + s.Version + "（与程序版本不一致，运行 rebang skill install 更新）"
		}
		rows = append(rows, []string{s.Agent, s.Dir, state})
	}
	if sts == nil {
		sts = []skill.Status{}
	}
	return output.Result{
		Data:    map[string]any{"version": buildinfo.Version, "skills": sts},
		Columns: []output.Column{{Title: "Agent"}, {Title: "目录"}, {Title: "状态"}},
		Rows:    rows,
	}
}

func init() {
	doctorChecks = append(doctorChecks, doctorSkill)
}

func doctorSkill(ctx context.Context, a *App) []Check {
	var cs []Check
	add := func(name, status, detail string) { cs = append(cs, Check{name, status, detail}) }
	if home, err := homeDir(); err == nil {
		var parts []string
		for _, ag := range skill.KnownAgents(home) {
			st := skill.StatusOf(ag, buildinfo.Version)
			if st.Installed {
				s := ag.Name + " " + st.Version
				if !st.UpToDate {
					s += "（版本不一致）"
				}
				parts = append(parts, s)
			}
		}
		if len(parts) == 0 {
			add("Agent Skill", "warn", "未安装，运行 rebang skill install")
		} else {
			add("Agent Skill", "ok", strings.Join(parts, "，"))
		}
	}
	return cs
}
