package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/output"
)

// SearchResultItem 是 search 命令输出的条目。
type SearchResultItem struct {
	Title      string `json:"title"`
	URL        string `json:"url"`
	MobileURL  string `json:"mobile_url,omitempty"`
	Snippet    string `json:"snippet,omitempty"`
	Source     string `json:"source"`
	SourceName string `json:"source_name,omitempty"`
	ItemKey    string `json:"item_key"`
	FirstSeen  string `json:"first_seen_at,omitempty"`
	LastSeen   string `json:"last_seen_at,omitempty"`
}

// SearchResult 是 search 命令的 JSON 输出。
type SearchResult struct {
	Query     string             `json:"query"`
	TimeRange string             `json:"time_range"`
	StartDate string             `json:"start_date,omitempty"`
	EndDate   string             `json:"end_date,omitempty"`
	Sources   []string           `json:"sources,omitempty"`
	Sort      string             `json:"sort,omitempty"`
	Total     int64              `json:"total"`
	HasMore   bool               `json:"has_more"`
	Items     []SearchResultItem `json:"items"`
}

// SearchWindow 把 --days 转换为服务端的 time_range 参数。
// 0 表示使用服务端默认值（普通用户当天，会员一年）。
func SearchWindow(days int, now time.Time) (timeRange, start, end string, err error) {
	switch days {
	case 0:
		return "", "", "", nil
	case 1:
		return "today", "", "", nil
	case 7, 30, 90, 365:
		return strconv.Itoa(days) + "d", "", "", nil
	}
	if days < 0 || days > 365 {
		return "", "", "", errs.Usage("--days 取值范围 1~365")
	}
	today := now.In(cst)
	return "custom", today.AddDate(0, 0, -(days - 1)).Format("2006-01-02"), today.Format("2006-01-02"), nil
}

// DoSearch 执行搜索并按 limit 翻页。
func DoSearch(ctx context.Context, c *api.Client, query string, days int, sources []string, sort string, limit int, now time.Time) (*SearchResult, error) {
	tr, start, end, err := SearchWindow(days, now)
	if err != nil {
		return nil, err
	}
	res := &SearchResult{Query: query, TimeRange: tr, StartDate: start, EndDate: end, Sources: sources, Sort: sort, Items: []SearchResultItem{}}
	if res.TimeRange == "" {
		res.TimeRange = "default"
	}
	names := map[string]string{}
	if _, opts, err := c.SearchOptions(ctx); err == nil {
		for _, o := range opts {
			names[o.Key] = o.Name
		}
	}
	size := limit
	if size > 50 {
		size = 50
	}
	for page := 1; ; page++ {
		p, err := c.Search(ctx, api.SearchQuery{Query: query, TabKeys: sources, TimeRange: tr, StartDate: start, EndDate: end, Sort: sort, Page: page, Size: size})
		if err != nil {
			return nil, api.MapError(err)
		}
		res.Total = p.Total
		for _, it := range p.List {
			if len(res.Items) >= limit {
				break
			}
			res.Items = append(res.Items, SearchResultItem{
				Title: it.Title, URL: it.URL, MobileURL: it.MobileURL, Snippet: it.Snippet,
				Source: it.TabKey, SourceName: names[it.TabKey], ItemKey: it.ItemKey,
				FirstSeen: formatUnix(it.CreatedAt), LastSeen: formatUnix(it.LastSeenAt),
			})
		}
		res.HasMore = p.HasMore
		if !p.HasMore || len(res.Items) >= limit || len(p.List) == 0 {
			break
		}
	}
	return res, nil
}

func newSearchCmd(a *App) *cobra.Command {
	var days, limit int
	var sources []string
	var sort string
	cmd := &cobra.Command{
		Use:   "search <关键词> [--days N] [--source …] [--limit N]",
		Short: "搜索热榜条目",
		Long: `搜索热榜条目。

普通用户只能搜索当天；搜索更早的时间、指定来源或按时间排序需要会员，
否则返回退出码 4（membership_required）。`,
		Example: `  rebang search 大模型
  rebang search 英伟达 --days 7 --source weibo --source zhihu -o json`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := strings.TrimSpace(strings.Join(args, " "))
			if q == "" {
				return errs.Usage("关键词不能为空")
			}
			if limit < 1 || limit > 200 {
				return errs.Usage("--limit 取值范围 1~200")
			}
			if sort != "" && sort != "relevance" && sort != "latest" {
				return errs.Usage("--sort 可选 relevance、latest")
			}
			var srcs []string
			for _, s := range sources {
				for _, p := range strings.Split(s, ",") {
					if p = strings.TrimSpace(p); p != "" {
						srcs = append(srcs, p)
					}
				}
			}
			res, err := DoSearch(cmd.Context(), a.Client(), q, days, srcs, sort, limit, a.Now())
			if err != nil {
				return err
			}
			rows := [][]string{}
			for i, it := range res.Items {
				src := it.SourceName
				if src == "" {
					src = it.Source
				}
				rows = append(rows, []string{strconv.Itoa(i + 1), it.Title, src, formatShort(it.LastSeen), it.URL})
			}
			return a.Writer().Write(output.Result{
				Data:    res,
				Columns: []output.Column{{Title: "#", Kind: output.KindRank}, {Title: "标题", Max: 60, Shrink: true}, {Title: "来源", Max: 12, Kind: output.KindAccent}, {Title: "最后在榜", Kind: output.KindDim}, {Title: "链接", Max: 60, Kind: output.KindDim}},
				Rows:    rows,
				Footer:  footer(a, fmt.Sprintf("共 %d 条结果", res.Total)),
				Markdown: func() string {
					var b strings.Builder
					fmt.Fprintf(&b, "搜索「%s」共 %d 条结果\n\n", q, res.Total)
					for i, it := range res.Items {
						src := it.SourceName
						if src == "" {
							src = it.Source
						}
						fmt.Fprintf(&b, "%d. %s — %s，%s\n", i+1, output.MDLink(it.Title, it.URL), src, formatShort(it.LastSeen))
					}
					return b.String()
				},
			})
		},
	}
	cmd.Flags().IntVar(&days, "days", 0, "搜索最近 N 天（1~365，默认当天，会员可用更长时间）")
	cmd.Flags().StringSliceVar(&sources, "source", nil, "限定来源（Tab KEY），可重复或用逗号分隔")
	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "最多返回条数（1~200）")
	cmd.Flags().StringVar(&sort, "sort", "", "排序：relevance（默认）或 latest")
	return cmd
}

func formatShort(rfc string) string {
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return ""
	}
	return t.In(cst).Format("2006-01-02 15:04")
}
