package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

// InsightBatchMax 是批量查询一句话总结单次最多携带的 item_key 数。
const InsightBatchMax = 50

// InsightShort 一句话总结。
type InsightShort struct {
	SummaryShort string `json:"summary_short"`
}

// Insight 单条 AI 总结。Detail 保留服务端原始 JSON，便于 -o json 原样输出；
// 展示时用 ParseDetail 解析已知分区，未知分区忽略。
type Insight struct {
	SummaryShort string          `json:"summary_short"`
	Detail       json.RawMessage `json:"detail"`
}

// InsightDetail 是 detail 中客户端认识的分区，任何分区都可能缺失。
type InsightDetail struct {
	Points     []string           `json:"points,omitempty"`
	Timeline   []InsightEvent     `json:"timeline,omitempty"`
	Viewpoints []InsightViewpoint `json:"viewpoints,omitempty"`
	Reactions  *InsightReactions  `json:"reactions,omitempty"`
}

// InsightEvent 经过中的一条，Time 可为空。
type InsightEvent struct {
	Time string `json:"time"`
	Text string `json:"text"`
}

// InsightViewpoint 一个观点，By 与 Quote 可为空。
type InsightViewpoint struct {
	Stance string `json:"stance"`
	Text   string `json:"text"`
	By     string `json:"by"`
	Quote  string `json:"quote"`
}

// InsightReactions 网友反应。
type InsightReactions struct {
	Mood   string         `json:"mood"`
	Voices []InsightVoice `json:"voices"`
}

// InsightVoice 一种代表性说法，Quote 可为空。
type InsightVoice struct {
	By    string `json:"by"`
	Text  string `json:"text"`
	Quote string `json:"quote"`
}

// ParseDetail 解析 detail 中已知的分区；为空或格式不符时返回零值。
func (in *Insight) ParseDetail() InsightDetail {
	var d InsightDetail
	if in == nil || len(in.Detail) == 0 {
		return d
	}
	if json.Unmarshal(in.Detail, &d) != nil {
		// 整体解析失败时逐个分区尝试，保留能用的部分。
		var parts map[string]json.RawMessage
		if json.Unmarshal(in.Detail, &parts) != nil {
			return InsightDetail{}
		}
		d = InsightDetail{}
		_ = json.Unmarshal(parts["points"], &d.Points)
		_ = json.Unmarshal(parts["timeline"], &d.Timeline)
		_ = json.Unmarshal(parts["viewpoints"], &d.Viewpoints)
		var r InsightReactions
		if json.Unmarshal(parts["reactions"], &r) == nil {
			d.Reactions = &r
		}
	}
	if d.Reactions != nil && d.Reactions.Mood == "" && len(d.Reactions.Voices) == 0 {
		d.Reactions = nil
	}
	return d
}

// InsightsBatch POST /v1/item/insights/batch（可选登录）。超过 50 个时分批请求，
// 返回有总结的 item_key；没有总结的不出现在结果中。任一批失败即返回错误。
func (c *Client) InsightsBatch(ctx context.Context, keys []string) (map[string]InsightShort, error) {
	out := map[string]InsightShort{}
	seen := make(map[string]bool, len(keys))
	uniq := make([]string, 0, len(keys))
	for _, k := range keys {
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		uniq = append(uniq, k)
	}
	for i := 0; i < len(uniq); i += InsightBatchMax {
		end := min(i+InsightBatchMax, len(uniq))
		var resp struct {
			Items map[string]InsightShort `json:"items"`
		}
		body := map[string][]string{"item_keys": uniq[i:end]}
		if err := c.do(ctx, http.MethodPost, "/item/insights/batch", nil, body, &resp); err != nil {
			return out, err
		}
		for k, v := range resp.Items {
			out[k] = v
		}
	}
	return out, nil
}

// Insight GET /v1/item/insight?item_key=（可选登录）。没有总结时服务端返回 404。
func (c *Client) Insight(ctx context.Context, itemKey string) (*Insight, error) {
	var out Insight
	err := c.do(ctx, http.MethodGet, "/item/insight", url.Values{"item_key": {itemKey}}, nil, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// IsNotFound 判断错误是否为服务端明确返回的“记录不存在”（业务码 1003）。
// 只有 HTTP 404 而没有业务码时（例如服务端版本较旧、没有这个接口）不算，避免误报为“暂无总结”。
func IsNotFound(err error) bool {
	var a *APIError
	return errors.As(err, &a) && a.Code == codeNotFound
}
