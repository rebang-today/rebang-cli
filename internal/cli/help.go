package cli

import (
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var defaultRe = regexp.MustCompile(` \(default ([^)]*)\)`)

// usageTemplate 是中文的帮助模板（结构与 cobra 默认模板一致，D-05）。
const usageTemplate = `用法：{{if .Runnable}}
  {{zhUse .UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} <命令>{{end}}{{if gt (len .Aliases) 0}}

别名：
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

示例：
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

命令：{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }}  {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}：{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }}  {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

其他命令：{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }}  {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

参数：
{{zhFlags .LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

全局参数：
{{zhFlags .InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

更多主题：{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

运行 "{{.CommandPath}} <命令> --help" 查看命令的详细用法。{{end}}
`

func init() {
	// 按使用顺序（菜单 → Tab → 榜单 → 搜索）而不是字母顺序列出命令。
	cobra.EnableCommandSorting = false
	cobra.AddTemplateFunc("zhUse", func(s string) string { return strings.Replace(s, " [flags]", " [参数]", 1) })
	cobra.AddTemplateFunc("zhFlags", func(s string) string { return defaultRe.ReplaceAllString(s, "（默认 $1）") })
}

// localizeHelp 为命令树设置中文帮助模板与内置命令、参数的说明。
func localizeHelp(root *cobra.Command) {
	root.SetUsageTemplate(usageTemplate)
	root.SetHelpCommand(&cobra.Command{
		Use:   "help [命令]",
		Short: "查看命令帮助",
		Run: func(c *cobra.Command, args []string) {
			cmd, _, e := c.Root().Find(args)
			if cmd == nil || e != nil {
				c.Printf("未知命令 %q\n", args)
				_ = c.Root().Usage()
				return
			}
			cmd.InitDefaultHelpFlag()
			_ = cmd.Help()
		},
	})
	root.CompletionOptions.HiddenDefaultCmd = false
	root.InitDefaultCompletionCmd()
	for _, c := range root.Commands() {
		if c.Name() == "completion" {
			c.Short = "生成 Shell 自动补全脚本"
			c.GroupID = "tool"
			for _, sub := range c.Commands() {
				sub.Short = "生成 " + sub.Name() + " 的自动补全脚本"
			}
		}
	}
	root.InitDefaultVersionFlag()
	if f := root.Flags().Lookup("version"); f != nil {
		f.Usage = "显示版本"
	}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		c.InitDefaultHelpFlag()
		if f := c.Flags().Lookup("help"); f != nil {
			f.Usage = "显示帮助"
		}
		for _, s := range c.Commands() {
			walk(s)
		}
	}
	walk(root)
}
