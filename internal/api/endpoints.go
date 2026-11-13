package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// Menu 菜单。
type Menu struct {
	ID   int64  `json:"id"`
	Name string `json:"menu_name"`
	Key  string `json:"menu_key"`
	Path string `json:"menu_path"`
}

// Tab 一级 Tab，Children 为二级 Tab。
type Tab struct {
	ID       int64    `json:"id"`
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	Type     string   `json:"type,omitempty"`
	Children []SubTab `json:"child"`
}

// SubTab 二级 Tab。
type SubTab struct {
	ID   int64  `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// Node 榜单节点。
type Node struct {
	NodeID        string `json:"node_id"`
	Name          string `json:"name"`
	Subtitle      string `json:"subtitle"`
	Desc          string `json:"desc"`
	SourceWebsite string `json:"source_website"`
	Type          string `json:"type,omitempty"`
}

// Menus GET /v1/menus。
func (c *Client) Menus(ctx context.Context) ([]Menu, error) {
	var out struct {
		Menus []Menu `json:"menus"`
	}
	err := c.do(ctx, http.MethodGet, "/menus", nil, nil, &out)
	return out.Menus, err
}

// MenuTabs GET /v1/tabs/menu?menu_id=。
func (c *Client) MenuTabs(ctx context.Context, menuID int64) ([]Tab, error) {
	var out struct {
		TabInfo []Tab `json:"tab_info"`
	}
	err := c.do(ctx, http.MethodGet, "/tabs/menu", url.Values{"menu_id": {strconv.FormatInt(menuID, 10)}}, nil, &out)
	return out.TabInfo, err
}

// TabChildren GET /v1/tabs/children?tab_key=。
func (c *Client) TabChildren(ctx context.Context, tabKey string) ([]SubTab, error) {
	var out struct {
		SubTabs []SubTab `json:"sub_tabs"`
	}
	err := c.do(ctx, http.MethodGet, "/tabs/children", url.Values{"tab_key": {tabKey}}, nil, &out)
	return out.SubTabs, err
}

// MenuNodes GET /v1/nodes/menu?menu_id=。
func (c *Client) MenuNodes(ctx context.Context, menuID int64) ([]Node, error) {
	var out struct {
		Nodes []Node `json:"nodes"`
	}
	err := c.do(ctx, http.MethodGet, "/nodes/menu", url.Values{"menu_id": {strconv.FormatInt(menuID, 10)}}, nil, &out)
	return out.Nodes, err
}

// FollowingNodes GET /v1/nodes/following（需登录）。
func (c *Client) FollowingNodes(ctx context.Context) ([]Node, error) {
	var out struct {
		Nodes []Node `json:"nodes"`
	}
	err := c.do(ctx, http.MethodGet, "/nodes/following", nil, nil, &out)
	return out.Nodes, err
}

// FollowingTabs GET /v1/tabs/following（需登录）。
func (c *Client) FollowingTabs(ctx context.Context) ([]Tab, error) {
	var out struct {
		TabInfo []Tab `json:"tab_info"`
	}
	err := c.do(ctx, http.MethodGet, "/tabs/following", nil, nil, &out)
	return out.TabInfo, err
}

// ListQuery 榜单查询条件：NodeID 或 Tab（+SubTab）二选一。
type ListQuery struct {
	NodeID   string
	Tab      string
	SubTab   string
	Page     int
	PageSize int
	Snapshot string
}

// Item 榜单条目的公共字段。不同布局的其余字段保留在 Extra 中。
type Item struct {
	ItemKey     string          `json:"item_key,omitempty"`
	Title       string          `json:"title"`
	URL         string          `json:"www_url"`
	MobileURL   string          `json:"mobile_url,omitempty"`
	Desc        string          `json:"desc,omitempty"`
	Label       string          `json:"label,omitempty"`
	PublishTime int64           `json:"publish_time,omitempty"`
	IsVideo     bool            `json:"is_video"`
	ExtraInfo   json.RawMessage `json:"extra_info,omitempty"`
}

// ContentAccess 榜单受会员限制时返回。
type ContentAccess struct {
	Mode         string `json:"mode"`
	Reason       string `json:"reason"`
	PreviewLimit int    `json:"preview_limit,omitempty"`
}

// ListPage 一页榜单。
type ListPage struct {
	SnapshotID      string         `json:"snapshot_id"`
	NextRefreshTime int64          `json:"next_refresh_time"`
	IsRanking       bool           `json:"is_ranking"`
	List            []Item         `json:"list"`
	HasMore         bool           `json:"has_more"`
	ContentAccess   *ContentAccess `json:"content_access,omitempty"`
}

// LatestList GET /v1/latest_list。
func (c *Client) LatestList(ctx context.Context, q ListQuery) (*ListPage, error) {
	v := url.Values{}
	if q.Page < 1 {
		q.Page = 1
	}
	v.Set("page", strconv.Itoa(q.Page))
	if q.PageSize > 0 {
		v.Set("page_size", strconv.Itoa(q.PageSize))
	}
	if q.NodeID != "" {
		v.Set("node_id", q.NodeID)
	}
	if q.Tab != "" {
		v.Set("tab", q.Tab)
	}
	if q.SubTab != "" {
		v.Set("sub_tab", q.SubTab)
	}
	if q.Snapshot != "" {
		v.Set("snapshot_id", q.Snapshot)
	}
	var out ListPage
	err := c.do(ctx, http.MethodGet, "/latest_list", v, nil, &out)
	return &out, err
}

// SearchQuery 搜索条件，对应服务端 ItemSearchReq。
type SearchQuery struct {
	Query     string
	TabKeys   []string
	TimeRange string
	StartDate string
	EndDate   string
	Sort      string
	Page      int
	Size      int
}

// SearchItem 搜索结果条目。
type SearchItem struct {
	ItemKey    string `json:"item_key"`
	Title      string `json:"title"`
	Snippet    string `json:"snippet"`
	TabKey     string `json:"tab_key"`
	URL        string `json:"www_url"`
	MobileURL  string `json:"mobile_url"`
	IsVideo    bool   `json:"is_video"`
	CreatedAt  int64  `json:"created_at"`
	LastSeenAt int64  `json:"last_seen_at"`
}

// SearchPage 一页搜索结果。
type SearchPage struct {
	Page    int          `json:"page"`
	Size    int          `json:"size"`
	Total   int64        `json:"total"`
	HasMore bool         `json:"has_more"`
	List    []SearchItem `json:"list"`
}

// Search GET /v1/item/search。
func (c *Client) Search(ctx context.Context, q SearchQuery) (*SearchPage, error) {
	v := url.Values{"q": {q.Query}}
	for _, k := range q.TabKeys {
		v.Add("tab_keys", k)
	}
	if q.TimeRange != "" {
		v.Set("time_range", q.TimeRange)
	}
	if q.StartDate != "" {
		v.Set("start_date", q.StartDate)
	}
	if q.EndDate != "" {
		v.Set("end_date", q.EndDate)
	}
	if q.Sort != "" {
		v.Set("sort", q.Sort)
	}
	if q.Page > 0 {
		v.Set("page", strconv.Itoa(q.Page))
	}
	if q.Size > 0 {
		v.Set("size", strconv.Itoa(q.Size))
	}
	var out SearchPage
	err := c.do(ctx, http.MethodGet, "/item/search", v, nil, &out)
	return &out, err
}

// SearchSource 可搜索的来源。
type SearchSource struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	IsVip int8   `json:"is_vip_tab"`
}

// SearchOptions GET /v1/item/search/options。
func (c *Client) SearchOptions(ctx context.Context) (timezone string, sources []SearchSource, err error) {
	var out struct {
		Timezone string         `json:"timezone"`
		Tabs     []SearchSource `json:"tabs"`
	}
	err = c.do(ctx, http.MethodGet, "/item/search/options", nil, nil, &out)
	return out.Timezone, out.Tabs, err
}

// Account 账号信息，对应服务端 UserAccount。
type Account struct {
	ID         int64  `json:"id"`
	CreatedAt  int64  `json:"created_at"`
	Email      string `json:"email"`
	EmailBound bool   `json:"email_bound"`
	IsVip      int8   `json:"is_vip"`
	VipEndAt   int64  `json:"vip_end_at"`
}

// Account GET /v1/user/account（需登录）。
func (c *Client) Account(ctx context.Context) (*Account, error) {
	var out Account
	err := c.do(ctx, http.MethodGet, "/user/account", nil, nil, &out)
	return &out, err
}
