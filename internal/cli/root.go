package cli

import (
	"github.com/spf13/cobra"

	"github.com/rebang-today/rebang-cli/internal/buildinfo"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/output"
	"github.com/rebang-today/rebang-cli/internal/update"
)

// 不受 min_version 限制的命令：升级自身、查看版本与诊断必须始终可用。
var upgradeExempt = map[string]bool{
	"update": true, "version": true, "doctor": true, "help": true, "completion": true, "skill": true,
}

// NewRootCmd 构造命令树。
func NewRootCmd(a *App) *cobra.Command {
	root := &cobra.Command{
		Use:   "rebang",
		Short: "今日热榜（rebang.today）命令行客户端",
		Long: `今日热榜（rebang.today）官方命令行客户端。

不带参数运行进入全屏交互界面；带子命令运行时输出结构化结果，
输出到终端时默认 table，被管道或 Agent 调用时默认 json。

退出码：0 成功，1 一般错误，2 参数错误，3 未登录或凭证失效，
4 需要会员，5 被限流，6 网络错误，7 需要升级。`,
		Example: `  rebang                         进入交互界面
  rebang auth login              浏览器授权登录
  rebang menus                   查看菜单
  rebang list --tab weibo        查看微博热搜
  rebang search 大模型           搜索热榜条目
  rebang list --tab zhihu -o json | jq '.items[0]'`,
		Version:       buildinfo.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			f, err := output.Resolve(a.outputFlag, a.Config.Output, a.StdoutTTY)
			if err != nil {
				return err
			}
			a.format = f
			if !exempt(cmd) {
				if err := update.EnforceMinVersion(buildinfo.Version); err != nil {
					return err
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !a.Interactive || a.RunTUI == nil {
				return cmd.Help()
			}
			return a.RunTUI(a)
		},
	}
	root.SetVersionTemplate("rebang {{.Version}}\n")
	root.SetFlagErrorFunc(cobraUsageErr)
	root.PersistentFlags().StringVarP(&a.outputFlag, "output", "o", "", "输出格式：json、md、table（默认终端 table，管道 json）")
	root.PersistentFlags().StringVar(&a.apiBaseFlag, "api-base", "", "接口地址，用于本地联调")
	_ = root.PersistentFlags().MarkHidden("api-base")

	root.AddGroup(&cobra.Group{ID: "query", Title: "查询"}, &cobra.Group{ID: "account", Title: "账号"}, &cobra.Group{ID: "tool", Title: "工具"})
	add := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	add("query", newMenusCmd(a), newTabsCmd(a), newNodesCmd(a), newListCmd(a), newSearchCmd(a), newSummaryCmd(a), newFollowingCmd(a), newOpenCmd(a))
	add("account", newAuthCmd(a), newAccountCmd(a))
	add("tool", newSkillCmd(a), newUpdateCmd(a), newDoctorCmd(a), newVersionCmd(a))
	localizeHelp(root)
	root.SetHelpCommandGroupID("tool")
	root.SetCompletionCommandGroupID("tool")
	return root
}

func exempt(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if upgradeExempt[c.Name()] {
			return true
		}
	}
	return false
}

// requireLogin 在没有任何凭证时直接返回退出码 3，避免无意义的请求。
func requireLogin() error {
	if tok, _ := tokenNow(); tok == "" {
		return errs.Unauthorized("尚未登录")
	}
	return nil
}
