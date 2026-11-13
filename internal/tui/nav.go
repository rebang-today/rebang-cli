package tui

import "github.com/rebang-today/rebang-cli/internal/api"

// listTarget 标识一个榜单。
type listTarget struct {
	node, tab, subTab string
	title             string
}

func (t listTarget) key() string { return t.node + "|" + t.tab + "|" + t.subTab }

// menuData 缓存一个菜单下的 Tab 与节点。
type menuData struct {
	tabs  []api.Tab
	nodes []api.Node
}

// srcKind 是左栏来源的类型。
type srcKind int

const (
	srcNone srcKind = iota
	srcTab
	srcNode
)

// source 是左栏的一项：一级 Tab，或（菜单没有 Tab 时的）节点。
type source struct {
	kind  srcKind
	label string
	tab   api.Tab
	node  api.Node
}

// subTab 是右栏顶部子榜单条的一项。
type subTab struct{ key, label string }

// followMenuID 是“订阅”菜单的编号。它不是服务端的菜单，数据来自用户订阅的 Tab。
const followMenuID int64 = -1

// monitoringTabKey 是订阅中“内容监控”Tab 的 key，它的列表不在公开的榜单接口里。
const monitoringTabKey = "monitoring"

const monitoringNotice = "内容监控暂不支持在命令行查看，请在今日热榜网站查看"

const followMenuName = "订阅"

// sources 返回菜单的左栏来源。
func sources(d *menuData) []source {
	if d == nil {
		return nil
	}
	var out []source
	for _, t := range d.tabs {
		out = append(out, source{kind: srcTab, label: t.Name, tab: t})
	}
	if len(d.tabs) == 0 {
		for _, n := range d.nodes {
			out = append(out, source{kind: srcNode, label: n.Name, node: n})
		}
	}
	return out
}

// key 用于记住每个来源上次的子榜单。
func (s source) key() string {
	if s.kind == srcNode {
		return "n:" + s.node.NodeID
	}
	return "t:" + s.tab.Key
}

// subs 返回子榜单条上显示的二级 Tab。与网站一致，只有一个时不显示（请求时仍带上它的 key）。
func (s source) subs() []subTab {
	if s.kind != srcTab || len(s.tab.Children) < 2 {
		return nil
	}
	out := make([]subTab, len(s.tab.Children))
	for i, c := range s.tab.Children {
		out[i] = subTab{c.Key, c.Name}
	}
	return out
}

// target 返回来源在第 sub 个二级 Tab 下对应的榜单。
func (s source) target(sub int) (listTarget, bool) {
	switch s.kind {
	case srcTab:
		t := listTarget{tab: s.tab.Key, title: s.tab.Name}
		if n := len(s.tab.Children); n > 0 {
			t.subTab = s.tab.Children[clampIdx(sub, n)].Key
		}
		return t, true
	case srcNode:
		return listTarget{node: s.node.NodeID, title: s.node.Name}, true
	}
	return listTarget{}, false
}

func clampIdx(i, n int) int {
	if i >= n {
		i = n - 1
	}
	if i < 0 {
		i = 0
	}
	return i
}

// window 在宽度 avail 内选出以 cur 为中心的一段连续项，两端被截断时各占 1 列显示 ‹ ›。
// 返回的 lo..hi 至少包含 cur；cur 本身放不下时由调用方截断。
func window(ws []int, cur, avail int) (lo, hi int) {
	n := len(ws)
	if n == 0 {
		return 0, -1
	}
	total := func(lo, hi int) int {
		s := 0
		for i := lo; i <= hi; i++ {
			s += ws[i]
		}
		if lo > 0 {
			s++
		}
		if hi < n-1 {
			s++
		}
		return s
	}
	if total(0, n-1) <= avail {
		return 0, n - 1
	}
	lo, hi = cur, cur
	for {
		grew := false
		if hi+1 < n && total(lo, hi+1) <= avail {
			hi++
			grew = true
		}
		if lo > 0 && total(lo-1, hi) <= avail {
			lo--
			grew = true
		}
		if !grew {
			return lo, hi
		}
	}
}

// unsupported 报告该来源的列表无法通过公开接口加载（目前只有内容监控）。
func (s source) unsupported() bool {
	return s.kind == srcTab && (s.tab.Key == monitoringTabKey || s.tab.Type == monitoringTabKey)
}
