package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/credentials"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/output"
)

func tokenNow() (string, credentials.Source) { return credentials.Token() }

func newMenusCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "menus",
		Short: "列出菜单（综合、科技、娱乐…）",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			menus, err := a.Client().Menus(cmd.Context())
			if err != nil {
				return api.MapError(err)
			}
			rows := make([][]string, 0, len(menus))
			for _, m := range menus {
				rows = append(rows, []string{strconv.FormatInt(m.ID, 10), m.Key, m.Name})
			}
			return a.Writer().Write(output.Result{
				Data:    map[string]any{"menus": menus},
				Columns: []output.Column{{Title: "ID", Kind: output.KindDim}, {Title: "KEY", Kind: output.KindAccent}, {Title: "名称"}},
				Rows:    rows,
				Footer:  footer(a, "查看菜单下的 Tab：rebang tabs --menu <ID>"),
			})
		},
	}
}

// footer 只在 table 与 md 模式下给人看的下一步提示。
func footer(a *App, s string) string {
	if a.Format() == output.Table {
		return "\n" + s
	}
	return ""
}

// resolveMenu 支持菜单 ID、KEY 或名称。
func resolveMenu(ctx context.Context, c *api.Client, s string) (api.Menu, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return api.Menu{}, errs.Usage("请用 --menu 指定菜单（ID、KEY 或名称），运行 rebang menus 查看")
	}
	if id, err := strconv.ParseInt(s, 10, 64); err == nil && id > 0 {
		return api.Menu{ID: id}, nil
	}
	menus, err := c.Menus(ctx)
	if err != nil {
		return api.Menu{}, api.MapError(err)
	}
	for _, m := range menus {
		if strings.EqualFold(m.Key, s) || m.Name == s {
			return m, nil
		}
	}
	return api.Menu{}, errs.New(errs.ExitUsage, errs.CodeInvalidArgument, fmt.Sprintf("找不到菜单 %q", s), "运行 rebang menus 查看可用菜单")
}

// MenuTab 是 tabs --all 输出的 Tab，Menus 为它出现的菜单 KEY。
type MenuTab struct {
	api.Tab
	Menus []string `json:"menus"`
}

// allTabs 汇总全部菜单下的 Tab。同一个 Tab 可能出现在多个菜单中，按 KEY 去重并保留首次出现的顺序，
// 让 Agent 一次拿到可用于 list --tab、search --source 的全部 KEY，不必先查菜单再逐个查 Tab。
func allTabs(ctx context.Context, c *api.Client) ([]MenuTab, error) {
	menus, err := c.Menus(ctx)
	if err != nil {
		return nil, api.MapError(err)
	}
	out := []MenuTab{}
	index := map[string]int{}
	for _, m := range menus {
		tabs, err := c.MenuTabs(ctx, m.ID)
		if err != nil {
			return nil, api.MapError(err)
		}
		for _, t := range tabs {
			if i, ok := index[t.Key]; ok {
				out[i].Menus = append(out[i].Menus, m.Key)
				if len(out[i].Children) == 0 {
					out[i].Children = t.Children
				}
				continue
			}
			index[t.Key] = len(out)
			out = append(out, MenuTab{Tab: t, Menus: []string{m.Key}})
		}
	}
	return out, nil
}

func subTabText(t api.Tab) string {
	subs := make([]string, 0, len(t.Children))
	for _, s := range t.Children {
		subs = append(subs, s.Key+"("+s.Name+")")
	}
	return strings.Join(subs, " ")
}

func newTabsCmd(a *App) *cobra.Command {
	var menu string
	var all bool
	cmd := &cobra.Command{
		Use:   "tabs --menu <id> | --all",
		Short: "列出菜单下的 Tab 与二级 Tab",
		Example: `  rebang tabs --menu tech
  rebang tabs --all -o json   # 全部菜单的 Tab，适合一次查出所有 Tab KEY`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := a.Client()
			if all {
				if menu != "" {
					return errs.Usage("--menu 与 --all 不能同时使用")
				}
				tabs, err := allTabs(cmd.Context(), c)
				if err != nil {
					return err
				}
				rows := [][]string{}
				for _, t := range tabs {
					rows = append(rows, []string{t.Key, t.Name, strings.Join(t.Menus, " "), subTabText(t.Tab)})
				}
				return a.Writer().Write(output.Result{
					Data:    map[string]any{"tabs": tabs},
					Columns: []output.Column{{Title: "TAB", Kind: output.KindAccent}, {Title: "名称"}, {Title: "菜单", Kind: output.KindDim}, {Title: "二级 TAB", Shrink: true, Kind: output.KindDim}},
					Rows:    rows,
					Footer:  footer(a, "查看榜单：rebang list --tab <TAB> [--sub-tab <二级 TAB>]"),
				})
			}
			m, err := resolveMenu(cmd.Context(), c, menu)
			if err != nil {
				return err
			}
			tabs, err := c.MenuTabs(cmd.Context(), m.ID)
			if err != nil {
				return api.MapError(err)
			}
			rows := [][]string{}
			for _, t := range tabs {
				rows = append(rows, []string{t.Key, t.Name, subTabText(t)})
			}
			return a.Writer().Write(output.Result{
				Data:    map[string]any{"menu_id": m.ID, "tabs": tabs},
				Columns: []output.Column{{Title: "TAB", Kind: output.KindAccent}, {Title: "名称"}, {Title: "二级 TAB", Shrink: true, Kind: output.KindDim}},
				Rows:    rows,
				Footer:  footer(a, "查看榜单：rebang list --tab <TAB> [--sub-tab <二级 TAB>]"),
			})
		},
	}
	cmd.Flags().StringVar(&menu, "menu", "", "菜单 ID、KEY 或名称")
	cmd.Flags().BoolVar(&all, "all", false, "列出全部菜单的 Tab（按 KEY 去重，menus 为所属菜单）")
	return cmd
}

// TabNode 是 Tab 下可直接查看的榜单。
type TabNode struct {
	Tab    string `json:"tab"`
	SubTab string `json:"sub_tab"`
	Name   string `json:"name"`
}

func newNodesCmd(a *App) *cobra.Command {
	var menu, tab string
	cmd := &cobra.Command{
		Use:   "nodes --menu <id> | --tab <key>",
		Short: "列出可直接查看的榜单节点",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (menu == "") == (tab == "") {
				return errs.Usage("--menu 与 --tab 必须且只能指定一个")
			}
			c := a.Client()
			if tab != "" {
				subs, err := c.TabChildren(cmd.Context(), tab)
				if err != nil {
					return api.MapError(err)
				}
				nodes := make([]TabNode, 0, len(subs))
				rows := [][]string{}
				for _, s := range subs {
					nodes = append(nodes, TabNode{Tab: tab, SubTab: s.Key, Name: s.Name})
					rows = append(rows, []string{tab, s.Key, s.Name})
				}
				return a.Writer().Write(output.Result{
					Data:    map[string]any{"tab": tab, "nodes": nodes},
					Columns: []output.Column{{Title: "TAB", Kind: output.KindDim}, {Title: "SUB-TAB", Kind: output.KindAccent}, {Title: "名称"}},
					Rows:    rows,
					Footer:  footer(a, "查看榜单：rebang list --tab <TAB> --sub-tab <SUB-TAB>"),
				})
			}
			m, err := resolveMenu(cmd.Context(), c, menu)
			if err != nil {
				return err
			}
			nodes, err := c.MenuNodes(cmd.Context(), m.ID)
			if err != nil {
				return api.MapError(err)
			}
			return a.Writer().Write(nodesResult(a, map[string]any{"menu_id": m.ID, "nodes": nodes}, nodes))
		},
	}
	cmd.Flags().StringVar(&menu, "menu", "", "菜单 ID、KEY 或名称")
	cmd.Flags().StringVar(&tab, "tab", "", "一级 Tab 的 KEY")
	return cmd
}

func nodesResult(a *App, data any, nodes []api.Node) output.Result {
	rows := [][]string{}
	for _, n := range nodes {
		rows = append(rows, []string{n.NodeID, n.Name, n.Subtitle})
	}
	return output.Result{
		Data:    data,
		Columns: []output.Column{{Title: "NODE", Kind: output.KindAccent}, {Title: "名称"}, {Title: "说明", Shrink: true, Kind: output.KindDim}},
		Rows:    rows,
		Footer:  footer(a, "查看榜单：rebang list --node <NODE>"),
	}
}

// ListItem 是 list 命令输出的条目。
type ListItem struct {
	Rank        int             `json:"rank"`
	Title       string          `json:"title"`
	URL         string          `json:"url"`
	MobileURL   string          `json:"mobile_url,omitempty"`
	Desc        string          `json:"desc,omitempty"`
	Heat        string          `json:"heat,omitempty"`
	Label       string          `json:"label,omitempty"`
	PublishedAt string          `json:"published_at,omitempty"`
	ItemKey     string          `json:"item_key,omitempty"`
	Extra       json.RawMessage `json:"extra_info,omitempty"`
}

// ListResult 是 list 命令的 JSON 输出。
type ListResult struct {
	NodeID        string             `json:"node_id,omitempty"`
	Tab           string             `json:"tab,omitempty"`
	SubTab        string             `json:"sub_tab,omitempty"`
	SnapshotID    string             `json:"snapshot_id,omitempty"`
	IsRanking     bool               `json:"is_ranking"`
	NextRefreshAt string             `json:"next_refresh_at,omitempty"`
	ContentAccess *api.ContentAccess `json:"content_access,omitempty"`
	Items         []ListItem         `json:"items"`
}

type listTarget struct {
	node, tab, subTab string
}

func (t listTarget) validate() error {
	if t.node == "" && t.tab == "" {
		return errs.Usage("请用 --node 或 --tab 指定榜单，运行 rebang nodes --menu <id> 查看")
	}
	if t.node != "" && t.tab != "" {
		return errs.Usage("--node 与 --tab 不能同时使用")
	}
	if t.subTab != "" && t.tab == "" {
		return errs.Usage("--sub-tab 需要配合 --tab 使用")
	}
	return nil
}

func addListTargetFlags(cmd *cobra.Command, t *listTarget) {
	cmd.Flags().StringVar(&t.node, "node", "", "节点 ID（rebang nodes 查看）")
	cmd.Flags().StringVar(&t.tab, "tab", "", "一级 Tab 的 KEY")
	cmd.Flags().StringVar(&t.subTab, "sub-tab", "", "二级 Tab 的 KEY")
}

// FetchList 拉取榜单，按 snapshot_id 翻页直到 limit。
// 只给出有二级 Tab 的一级 Tab 时，默认使用第一个二级 Tab。
func FetchList(ctx context.Context, c *api.Client, node, tab, subTab string, limit int) (*ListResult, error) {
	if tab != "" && subTab == "" {
		subs, err := c.TabChildren(ctx, tab)
		if err != nil {
			return nil, api.MapError(err)
		}
		if len(subs) > 0 {
			subTab = subs[0].Key
		}
	}
	res := &ListResult{NodeID: node, Tab: tab, SubTab: subTab, Items: []ListItem{}}
	q := api.ListQuery{NodeID: node, Tab: tab, SubTab: subTab, Page: 1, PageSize: 50}
	if limit < 50 {
		q.PageSize = limit
	}
	for {
		page, err := c.LatestList(ctx, q)
		if err != nil {
			return nil, api.MapError(err)
		}
		if q.Page == 1 {
			res.SnapshotID = page.SnapshotID
			res.IsRanking = page.IsRanking
			res.NextRefreshAt = formatUnix(page.NextRefreshTime)
			res.ContentAccess = page.ContentAccess
		}
		for _, it := range page.List {
			if len(res.Items) >= limit {
				break
			}
			res.Items = append(res.Items, ListItem{
				Rank:        len(res.Items) + 1,
				Title:       it.Title,
				URL:         it.URL,
				MobileURL:   it.MobileURL,
				Desc:        it.Desc,
				Heat:        Heat(it.ExtraInfo),
				Label:       it.Label,
				PublishedAt: formatUnix(it.PublishTime),
				ItemKey:     it.ItemKey,
				Extra:       it.ExtraInfo,
			})
		}
		if !page.HasMore || len(res.Items) >= limit || len(page.List) == 0 || page.SnapshotID == "" {
			break
		}
		q.Page++
		q.Snapshot = page.SnapshotID
	}
	if len(res.Items) == 0 && res.ContentAccess != nil && res.ContentAccess.Mode == "locked" {
		return res, errs.Membership("该榜单仅对会员开放")
	}
	return res, nil
}

// heatKeys 是 extra_info 中常见的热度字段，按优先级排列。
var heatKeys = []string{"hot_value", "heat", "hot", "hot_num", "hot_score", "hot_total", "view", "views"}

// Heat 从 extra_info 中提取热度展示值。
func Heat(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	// 多数榜单把展示文案放在 display_text_items，例如“1290万热度”。
	if items, ok := m["display_text_items"].([]any); ok {
		for _, it := range items {
			if obj, ok := it.(map[string]any); ok {
				if s := scalar(obj["text"]); s != "" {
					return s
				}
			}
		}
	}
	for _, k := range heatKeys {
		if v, ok := m[k]; ok {
			if s := scalar(v); s != "" {
				return s
			}
		}
	}
	for k, v := range m {
		if strings.Contains(k, "hot") || strings.Contains(k, "heat") {
			if s := scalar(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func scalar(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		if x == 0 {
			return ""
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return ""
}

func newListCmd(a *App) *cobra.Command {
	var t listTarget
	var limit int
	cmd := &cobra.Command{
		Use:   "list --node <id> | --tab <key> [--sub-tab <key>]",
		Short: "查看榜单",
		Example: `  rebang list --tab weibo
  rebang list --tab github --sub-tab daily -n 10
  rebang list --node <NODE> -o md`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := t.validate(); err != nil {
				return err
			}
			if limit < 1 || limit > 500 {
				return errs.Usage("--limit 取值范围 1~500")
			}
			res, err := FetchList(cmd.Context(), a.Client(), t.node, t.tab, t.subTab, limit)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, it := range res.Items {
				rows = append(rows, []string{strconv.Itoa(it.Rank), it.Title, it.Heat, it.URL})
			}
			ft := ""
			if res.ContentAccess != nil && res.ContentAccess.Mode == "preview" {
				ft = fmt.Sprintf("非会员仅可预览前 %d 条，开通会员查看完整榜单。", res.ContentAccess.PreviewLimit)
			}
			r := output.Result{
				Data:    res,
				Columns: []output.Column{{Title: "#", Kind: output.KindRank}, {Title: "标题", Max: 60, Shrink: true}, {Title: "热度", Max: 12, Kind: output.KindAccent}, {Title: "链接", Max: 60, Kind: output.KindDim}},
				Rows:    rows,
				Markdown: func() string {
					var b strings.Builder
					for _, it := range res.Items {
						fmt.Fprintf(&b, "%d. %s", it.Rank, output.MDLink(it.Title, it.URL))
						if it.Heat != "" {
							fmt.Fprintf(&b, "（%s）", it.Heat)
						}
						b.WriteString("\n")
					}
					if ft != "" {
						b.WriteString("\n> " + ft + "\n")
					}
					return b.String()
				},
			}
			if a.Format() == output.Table {
				r.Footer = "\n" + strings.TrimSpace(ft+"\n用浏览器打开第 N 条：rebang open N "+t.flagString())
			}
			return a.Writer().Write(r)
		},
	}
	addListTargetFlags(cmd, &t)
	cmd.Flags().IntVarP(&limit, "limit", "n", 30, "最多返回条数（1~500）")
	return cmd
}

func (t listTarget) flagString() string {
	if t.node != "" {
		return "--node " + t.node
	}
	s := "--tab " + t.tab
	if t.subTab != "" {
		s += " --sub-tab " + t.subTab
	}
	return s
}
