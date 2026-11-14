package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// MonitorHitsPageMax 是 /v1/monitoring/hits 单页最多条数。
const MonitorHitsPageMax = 30

// 监控计划状态。
const (
	MonitorActive            = "active"
	MonitorPaused            = "paused"
	MonitorMembershipExpired = "membership_expired"
)

// MonitorPlan 内容监控计划（只读取展示需要的字段）。
type MonitorPlan struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Keywords []string `json:"keywords"`
	Status   string   `json:"status"`
}

// MonitorPlanUnread 单个计划的未读数。
type MonitorPlanUnread struct {
	PlanID      int64 `json:"plan_id"`
	UnreadCount int64 `json:"unread_count"`
}

// MonitorUnread 是 /v1/monitoring/unread-counts 的结果。
type MonitorUnread struct {
	All   int64               `json:"all"`
	Plans []MonitorPlanUnread `json:"plans"`
}

// Plan 返回某个计划的未读数，没有时为 0。
func (u *MonitorUnread) Plan(id int64) int64 {
	if u == nil {
		return 0
	}
	for _, p := range u.Plans {
		if p.PlanID == id {
			return p.UnreadCount
		}
	}
	return 0
}

// MonitorPlanMatch 一条命中内容命中的计划与关键词。
type MonitorPlanMatch struct {
	PlanID          int64    `json:"plan_id"`
	PlanName        string   `json:"plan_name"`
	Keywords        []string `json:"keywords"`
	MatchedKeywords []string `json:"matched_keywords"`
	WholeWord       bool     `json:"whole_word"`
}

// MonitorHit 一条命中内容。时间为 Unix 秒。
type MonitorHit struct {
	ItemKey       string             `json:"item_key"`
	Title         string             `json:"title"`
	Description   string             `json:"description"`
	SourceKey     string             `json:"source_key"`
	SourceName    string             `json:"source_name"`
	WwwURL        string             `json:"www_url"`
	MobileURL     string             `json:"mobile_url"`
	ItemCreatedAt int64              `json:"item_created_at"`
	MatchedAt     int64              `json:"matched_at"`
	IsRead        bool               `json:"is_read"`
	PlanMatches   []MonitorPlanMatch `json:"plan_matches"`
}

// URL 返回原文链接，优先网页版。
func (h *MonitorHit) URL() string {
	if h.WwwURL != "" {
		return h.WwwURL
	}
	return h.MobileURL
}

// MatchedKeywords 汇总所有命中计划的命中词（去重，保持顺序）。
func (h *MonitorHit) MatchedKeywords() []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range h.PlanMatches {
		for _, k := range m.MatchedKeywords {
			if k != "" && !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

// MonitorHits 是 /v1/monitoring/hits 的一页结果。
type MonitorHits struct {
	Items      []MonitorHit `json:"items"`
	NextCursor string       `json:"next_cursor"`
	HasMore    bool         `json:"has_more"`
	RevisionAt int64        `json:"revision_at"`
	RevisionID int64        `json:"revision_id"`
	NewCount   int64        `json:"new_count"`
}

// MonitorHitsQuery 是命中列表的查询条件。PlanID 为 0 表示全部计划。
type MonitorHitsQuery struct {
	PlanID     int64
	Cursor     string
	UnreadOnly bool
	PageSize   int
}

// MonitorPlans GET /v1/monitoring/plans（需登录），按服务端顺序返回。
func (c *Client) MonitorPlans(ctx context.Context) ([]MonitorPlan, error) {
	var out []MonitorPlan
	err := c.do(ctx, http.MethodGet, "/monitoring/plans", nil, nil, &out)
	return out, err
}

// MonitorUnreadCounts GET /v1/monitoring/unread-counts（需登录）。
func (c *Client) MonitorUnreadCounts(ctx context.Context) (*MonitorUnread, error) {
	var out MonitorUnread
	if err := c.do(ctx, http.MethodGet, "/monitoring/unread-counts", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MonitorHitsPage GET /v1/monitoring/hits（需登录），返回一页命中内容。
func (c *Client) MonitorHitsPage(ctx context.Context, q MonitorHitsQuery) (*MonitorHits, error) {
	size := q.PageSize
	if size <= 0 || size > MonitorHitsPageMax {
		size = MonitorHitsPageMax
	}
	v := url.Values{"page_size": {strconv.Itoa(size)}}
	if q.PlanID > 0 {
		v.Set("plan_id", strconv.FormatInt(q.PlanID, 10))
	}
	if q.Cursor != "" {
		v.Set("cursor", q.Cursor)
	}
	if q.UnreadOnly {
		v.Set("read_status", "unread")
	}
	var out MonitorHits
	if err := c.do(ctx, http.MethodGet, "/monitoring/hits", v, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MonitorMarkRead POST /v1/monitoring/read（需登录），把一条命中内容标为已读，返回最新的未读数。
func (c *Client) MonitorMarkRead(ctx context.Context, itemKey string) (*MonitorUnread, error) {
	var out MonitorUnread
	if err := c.do(ctx, http.MethodPost, "/monitoring/read", nil, map[string]string{"item_key": itemKey}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
