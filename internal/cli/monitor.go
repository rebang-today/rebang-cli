package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/output"
)

// MonitorPlanView 是 monitor plans 输出的一个计划。
type MonitorPlanView struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Keywords    []string `json:"keywords"`
	Status      string   `json:"status"`
	UnreadCount int64    `json:"unread_count"`
}

// MonitorPlansResult 是 monitor plans 的 JSON 输出。
type MonitorPlansResult struct {
	UnreadAll int64             `json:"unread_all"`
	Plans     []MonitorPlanView `json:"plans"`
}

// MonitorMatch 是一条命中内容命中的计划与命中词。
type MonitorMatch struct {
	PlanID          int64    `json:"plan_id"`
	PlanName        string   `json:"plan_name"`
	MatchedKeywords []string `json:"matched_keywords"`
}

// MonitorHitView 是 monitor hits 输出的一条命中内容。
type MonitorHitView struct {
	ItemKey     string         `json:"item_key"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Source      string         `json:"source"`
	SourceName  string         `json:"source_name,omitempty"`
	URL         string         `json:"url"`
	MobileURL   string         `json:"mobile_url,omitempty"`
	MatchedAt   string         `json:"matched_at"`
	PublishedAt string         `json:"published_at,omitempty"`
	IsRead      bool           `json:"is_read"`
	Matches     []MonitorMatch `json:"matches"`
}

// MonitorHitsResult 是 monitor hits 的 JSON 输出。
type MonitorHitsResult struct {
	PlanID     int64            `json:"plan_id,omitempty"`
	UnreadOnly bool             `json:"unread_only"`
	HasMore    bool             `json:"has_more"`
	Items      []MonitorHitView `json:"items"`
}

// MonitorStatusText 返回计划状态的中文说明。
func MonitorStatusText(s string) string {
	switch s {
	case api.MonitorActive:
		return "监控中"
	case api.MonitorPaused:
		return "已暂停"
	case api.MonitorMembershipExpired:
		return "会员过期"
	}
	return s
}

func newMonitorCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "内容监控：查看计划与命中内容（需登录）",
		Long: "查看内容监控的计划与命中内容（只读）。\n\n" +
			"创建、编辑、暂停、删除计划与通知设置请在网站上操作。命令行只读取，不会把命中内容标为已读。",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newMonitorPlansCmd(a), newMonitorHitsCmd(a))
	return cmd
}

func newMonitorPlansCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "plans",
		Short: "列出监控计划与未读数",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireLogin(); err != nil {
				return err
			}
			c := a.Client()
			plans, err := c.MonitorPlans(cmd.Context())
			if err != nil {
				return api.MapError(err)
			}
			unread, err := c.MonitorUnreadCounts(cmd.Context())
			if err != nil {
				return api.MapError(err)
			}
			res := MonitorPlansResult{UnreadAll: unread.All, Plans: []MonitorPlanView{}}
			rows := [][]string{}
			for _, p := range plans {
				kw := p.Keywords
				if kw == nil {
					kw = []string{}
				}
				v := MonitorPlanView{ID: p.ID, Name: p.Name, Keywords: kw, Status: p.Status, UnreadCount: unread.Plan(p.ID)}
				res.Plans = append(res.Plans, v)
				rows = append(rows, []string{itoa(v.ID), v.Name, strings.Join(v.Keywords, "、"), MonitorStatusText(v.Status), itoa(v.UnreadCount)})
			}
			note := fmt.Sprintf("共 %d 个计划，未读 %d 条；查看命中：rebang monitor hits --plan <ID>", len(res.Plans), res.UnreadAll)
			if len(res.Plans) == 0 {
				note = "还没有监控计划，可在网站创建：" + strings.TrimRight(a.Config.Web(), "/") + "/monitoring"
			}
			return a.Writer().Write(output.Result{
				Data:    res,
				Columns: []output.Column{{Title: "ID", Kind: output.KindAccent}, {Title: "名称", Max: 24}, {Title: "关键词", Max: 40, Shrink: true}, {Title: "状态", Kind: output.KindDim}, {Title: "未读", Kind: output.KindRank}},
				Rows:    rows,
				Footer:  footer(a, note),
				Markdown: func() string {
					var b strings.Builder
					fmt.Fprintf(&b, "内容监控：%d 个计划，未读 %d 条\n\n", len(res.Plans), res.UnreadAll)
					for _, p := range res.Plans {
						fmt.Fprintf(&b, "- %s（ID %d，%s，未读 %d）：%s\n", p.Name, p.ID, MonitorStatusText(p.Status), p.UnreadCount, strings.Join(p.Keywords, "、"))
					}
					return b.String()
				},
			})
		},
	}
}

// FetchMonitorHits 按 cursor 翻页，凑够 limit 条命中内容。
func FetchMonitorHits(ctx context.Context, c *api.Client, planID int64, unread bool, limit int) (*MonitorHitsResult, error) {
	res := &MonitorHitsResult{PlanID: planID, UnreadOnly: unread, Items: []MonitorHitView{}}
	q := api.MonitorHitsQuery{PlanID: planID, UnreadOnly: unread, PageSize: min(limit, api.MonitorHitsPageMax)}
	for {
		p, err := c.MonitorHitsPage(ctx, q)
		if err != nil {
			return nil, api.MapError(err)
		}
		for _, h := range p.Items {
			if len(res.Items) >= limit {
				res.HasMore = true
				return res, nil
			}
			res.Items = append(res.Items, hitView(h))
		}
		if !p.HasMore || p.NextCursor == "" || len(p.Items) == 0 {
			return res, nil
		}
		if len(res.Items) >= limit {
			res.HasMore = true
			return res, nil
		}
		q.Cursor = p.NextCursor
	}
}

func hitView(h api.MonitorHit) MonitorHitView {
	v := MonitorHitView{
		ItemKey: h.ItemKey, Title: h.Title, Description: h.Description, Source: h.SourceKey, SourceName: h.SourceName,
		URL: h.URL(), MatchedAt: formatUnix(h.MatchedAt), PublishedAt: formatUnix(h.ItemCreatedAt), IsRead: h.IsRead, Matches: []MonitorMatch{},
	}
	if h.WwwURL != "" && h.MobileURL != h.WwwURL {
		v.MobileURL = h.MobileURL
	}
	for _, m := range h.PlanMatches {
		kw := m.MatchedKeywords
		if kw == nil {
			kw = []string{}
		}
		v.Matches = append(v.Matches, MonitorMatch{PlanID: m.PlanID, PlanName: m.PlanName, MatchedKeywords: kw})
	}
	return v
}

// matchText 把命中计划与命中词拼成“计划：词、词；计划：词”。
func matchText(ms []MonitorMatch) string {
	parts := make([]string, 0, len(ms))
	for _, m := range ms {
		s := m.PlanName
		if len(m.MatchedKeywords) > 0 {
			s += "：" + strings.Join(m.MatchedKeywords, "、")
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "；")
}

func newMonitorHitsCmd(a *App) *cobra.Command {
	var plan string
	var unread bool
	var limit int
	cmd := &cobra.Command{
		Use:   "hits [--plan <ID>] [--unread] [-n N]",
		Short: "查看命中内容（只读，不标记已读）",
		Example: `  rebang monitor hits
  rebang monitor hits --plan 12 -n 50
  rebang monitor hits --unread -o json   # 定时拉取新命中`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if limit < 1 || limit > 100 {
				return errs.Usage("--limit 取值范围 1~100")
			}
			var planID int64
			if plan != "" {
				id, err := strconv.ParseInt(strings.TrimSpace(plan), 10, 64)
				if err != nil || id <= 0 {
					return errs.Usage("--plan 应为计划 ID（正整数），用 rebang monitor plans 查看")
				}
				planID = id
			}
			if err := requireLogin(); err != nil {
				return err
			}
			res, err := FetchMonitorHits(cmd.Context(), a.Client(), planID, unread, limit)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for i, it := range res.Items {
				dot := ""
				if !it.IsRead {
					dot = "●"
				}
				src := it.SourceName
				if src == "" {
					src = it.Source
				}
				rows = append(rows, []string{strconv.Itoa(i + 1), dot, it.Title, src, matchText(it.Matches), formatShort(it.MatchedAt), it.ItemKey, it.URL})
			}
			note := fmt.Sprintf("共 %d 条", len(res.Items))
			if res.HasMore {
				note += "，还有更多（-n 最多 100）"
			}
			if len(res.Items) == 0 {
				note = "没有命中内容"
				if unread {
					note = "没有未读的命中内容"
				}
			}
			return a.Writer().Write(output.Result{
				Data: res,
				Columns: []output.Column{
					{Title: "#", Kind: output.KindRank}, {Title: "未读", Kind: output.KindAccent}, {Title: "标题", Max: 50, Shrink: true}, {Title: "来源", Max: 10, Kind: output.KindAccent},
					{Title: "命中", Max: 24, Shrink: true}, {Title: "命中时间", Kind: output.KindDim}, {Title: "item_key", Kind: output.KindDim}, {Title: "链接", Max: 50, Kind: output.KindDim},
				},
				Rows:   rows,
				Footer: footer(a, note),
				Markdown: func() string {
					var b strings.Builder
					fmt.Fprintf(&b, "内容监控命中 %d 条\n\n", len(res.Items))
					for i, it := range res.Items {
						src := it.SourceName
						if src == "" {
							src = it.Source
						}
						state := ""
						if !it.IsRead {
							state = "（未读）"
						}
						fmt.Fprintf(&b, "%d. %s%s — %s，%s，命中 %s，item_key `%s`\n", i+1, output.MDLink(it.Title, it.URL), state, src, formatShort(it.MatchedAt), matchText(it.Matches), it.ItemKey)
					}
					return b.String()
				},
			})
		},
	}
	cmd.Flags().StringVar(&plan, "plan", "", "只看某个计划（计划 ID）")
	cmd.Flags().BoolVar(&unread, "unread", false, "只看未读")
	cmd.Flags().IntVarP(&limit, "limit", "n", 30, "最多返回条数（1~100）")
	return cmd
}
