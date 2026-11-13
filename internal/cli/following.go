package cli

import (
	"github.com/spf13/cobra"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/output"
)

// FollowingResult 是 following 命令的 JSON 输出。
type FollowingResult struct {
	Nodes []api.Node `json:"nodes"`
	Tabs  []api.Tab  `json:"tabs"`
}

func newFollowingCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "following",
		Short: "我的关注（需登录）",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireLogin(); err != nil {
				return err
			}
			c := a.Client()
			nodes, err := c.FollowingNodes(cmd.Context())
			if err != nil {
				return api.MapError(err)
			}
			tabs, err := c.FollowingTabs(cmd.Context())
			if err != nil {
				return api.MapError(err)
			}
			if nodes == nil {
				nodes = []api.Node{}
			}
			if tabs == nil {
				tabs = []api.Tab{}
			}
			rows := [][]string{}
			for _, n := range nodes {
				rows = append(rows, []string{"节点", n.NodeID, n.Name, "rebang list --node " + n.NodeID})
			}
			for _, t := range tabs {
				rows = append(rows, []string{"Tab", t.Key, t.Name, "rebang list --tab " + t.Key})
			}
			return a.Writer().Write(output.Result{
				Data:    FollowingResult{Nodes: nodes, Tabs: tabs},
				Columns: []output.Column{{Title: "类型", Kind: output.KindDim}, {Title: "ID", Kind: output.KindAccent}, {Title: "名称"}, {Title: "查看", Kind: output.KindDim}},
				Rows:    rows,
			})
		},
	}
}
