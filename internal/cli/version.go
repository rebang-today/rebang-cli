package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/rebang-today/rebang-cli/internal/buildinfo"
	"github.com/rebang-today/rebang-cli/internal/output"
)

// VersionInfo 是 version 命令的 JSON 输出。
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	Platform  string `json:"platform"`
	GoVersion string `json:"go_version"`
}

func newVersionCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "版本信息",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			v := VersionInfo{buildinfo.Version, buildinfo.Commit, buildinfo.Date, buildinfo.Platform(), runtime.Version()}
			return a.Writer().Write(output.Result{
				Data:     v,
				Columns:  []output.Column{{Title: "版本"}, {Title: "提交"}, {Title: "构建时间"}, {Title: "平台"}},
				Rows:     [][]string{{v.Version, v.Commit, v.Date, v.Platform}},
				Markdown: func() string { return fmt.Sprintf("rebang %s (%s, %s)", v.Version, v.Platform, v.Commit) },
			})
		},
	}
}
