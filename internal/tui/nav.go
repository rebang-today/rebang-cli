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
	srcFollowing srcKind = iota
	srcTab
	srcNode
)

// source 是左栏的一项：我的关注、一级 Tab，或（菜单没有 Tab 时的）节点。
type source struct {
	kind  srcKind
	label string
	tab   api.Tab
	node  api.Node
}

// subTab 是右栏顶部子榜单条的一项。
type subTab struct{ key, label string }

const followingLabel = "我的关注"

// sources 返回菜单的左栏来源，第一项固定为“我的关注”。
func sources(d *menuData) []source {
	out := []source{{kind: srcFollowing, label: followingLabel}}
	if d == nil {
		return out
	}
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
	switch s.kind {
	case srcTab:
		return "t:" + s.tab.Key
	case srcNode:
		return "n:" + s.node.NodeID
	}
	return "following"
}

// subs 返回来源的子榜单：Tab 的二级 Tab，或“我的关注”中关注的节点。
func (s source) subs(following []api.Node) []subTab {
	var out []subTab
	switch s.kind {
	case srcTab:
		for _, c := range s.tab.Children {
			out = append(out, subTab{c.Key, c.Name})
		}
	case srcFollowing:
		for _, n := range following {
			out = append(out, subTab{n.NodeID, n.Name})
		}
	}
	return out
}

// target 返回来源在第 sub 个子榜单下对应的榜单；“我的关注”为空时返回 false。
func (s source) target(sub int, following []api.Node) (listTarget, bool) {
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
	if s.kind != srcFollowing || len(following) == 0 {
		return listTarget{}, false
	}
	n := following[clampIdx(sub, len(following))]
	return listTarget{node: n.NodeID, title: n.Name}, true
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
