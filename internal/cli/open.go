package cli

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/output"
)

func newOpenCmd(a *App) *cobra.Command {
	var t listTarget
	var printOnly, mobile bool
	cmd := &cobra.Command{
		Use:   "open <链接 | 序号>",
		Short: "用浏览器打开条目",
		Long: `用浏览器打开条目。

参数可以是链接，也可以是榜单中的序号（需配合 --node 或 --tab）。`,
		Example: `  rebang open https://example.com/a
  rebang open 3 --node <NODE>
  rebang open 1 --tab weibo --print`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := strings.TrimSpace(args[0])
			url := ""
			title := ""
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
				url = target
			} else {
				n, err := strconv.Atoi(target)
				if err != nil || n < 1 {
					return errs.Usage("参数应为链接或从 1 开始的序号")
				}
				if err := t.validate(); err != nil {
					return err
				}
				res, err := FetchList(cmd.Context(), a.Client(), t.node, t.tab, t.subTab, n)
				if err != nil {
					return err
				}
				if len(res.Items) < n {
					return errs.Usage("榜单只有 %d 条", len(res.Items))
				}
				it := res.Items[n-1]
				url, title = it.URL, it.Title
				if mobile && it.MobileURL != "" {
					url = it.MobileURL
				}
			}
			opened := false
			if !printOnly {
				if err := a.OpenURL(url); err != nil {
					return errs.New(errs.ExitGeneral, errs.CodeGeneral, "无法打开浏览器: "+err.Error(), "使用 --print 只输出链接")
				}
				opened = true
			}
			return a.Writer().Write(output.Result{
				Data:     map[string]any{"url": url, "title": title, "opened": opened},
				Columns:  []output.Column{{Title: "标题"}, {Title: "链接"}},
				Rows:     [][]string{{title, url}},
				Markdown: func() string { return output.MDLink(firstNonEmpty(title, url), url) },
			})
		},
	}
	addListTargetFlags(cmd, &t)
	cmd.Flags().BoolVar(&printOnly, "print", false, "只输出链接，不打开浏览器")
	cmd.Flags().BoolVar(&mobile, "mobile", false, "优先使用移动版链接")
	return cmd
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
