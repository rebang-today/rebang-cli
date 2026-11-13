package tui

import "github.com/rebang-today/rebang-cli/internal/api"

// entryKind 左栏条目类型。
type entryKind int

const (
	kindMenu entryKind = iota
	kindTab
	kindSubTab
	kindNode
	kindFollowing
)

// entry 是左栏树的一行。
type entry struct {
	kind   entryKind
	level  int
	label  string
	menuID int64
	tab    string
	subTab string
	nodeID string
	// expandable 表示可以展开子级。
	expandable bool
	expanded   bool
}

// target 返回条目对应的榜单，非榜单返回 false。
func (e entry) target() (listTarget, bool) {
	switch e.kind {
	case kindNode:
		return listTarget{node: e.nodeID, title: e.label}, true
	case kindTab:
		return listTarget{tab: e.tab, title: e.label}, true
	case kindSubTab:
		return listTarget{tab: e.tab, subTab: e.subTab, title: e.label}, true
	}
	return listTarget{}, false
}

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

// tree 维护菜单 → Tab → 二级 Tab 的展开状态。
type tree struct {
	menus      []api.Menu
	data       map[int64]*menuData
	expanded   map[string]bool
	following  []api.Node
	loggedIn   bool
	followOpen bool
}

func newTree() *tree {
	return &tree{data: map[int64]*menuData{}, expanded: map[string]bool{}}
}

func menuKey(id int64) string            { return "m" + itoa(id) }
func tabKey(menu int64, k string) string { return "t" + itoa(menu) + "/" + k }

// rows 展开为可见行。
func (t *tree) rows() []entry {
	var out []entry
	if t.loggedIn {
		out = append(out, entry{kind: kindFollowing, label: "我的关注", expandable: true, expanded: t.followOpen})
		if t.followOpen {
			for _, n := range t.following {
				out = append(out, entry{kind: kindNode, level: 1, label: n.Name, nodeID: n.NodeID})
			}
		}
	}
	for _, m := range t.menus {
		open := t.expanded[menuKey(m.ID)]
		out = append(out, entry{kind: kindMenu, label: m.Name, menuID: m.ID, expandable: true, expanded: open})
		if !open {
			continue
		}
		d := t.data[m.ID]
		if d == nil {
			continue
		}
		for _, tb := range d.tabs {
			tk := tabKey(m.ID, tb.Key)
			hasChild := len(tb.Children) > 0
			topen := t.expanded[tk]
			out = append(out, entry{kind: kindTab, level: 1, label: tb.Name, menuID: m.ID, tab: tb.Key, expandable: hasChild, expanded: topen})
			if hasChild && topen {
				for _, s := range tb.Children {
					out = append(out, entry{kind: kindSubTab, level: 2, label: s.Name, menuID: m.ID, tab: tb.Key, subTab: s.Key})
				}
			}
		}
		if len(d.tabs) == 0 {
			for _, n := range d.nodes {
				out = append(out, entry{kind: kindNode, level: 1, label: n.Name, menuID: m.ID, nodeID: n.NodeID})
			}
		}
	}
	return out
}

// toggle 切换展开状态，返回是否需要加载菜单数据。
func (t *tree) toggle(e entry) (needLoad bool) {
	switch e.kind {
	case kindFollowing:
		t.followOpen = !t.followOpen
	case kindMenu:
		k := menuKey(e.menuID)
		t.expanded[k] = !t.expanded[k]
		return t.expanded[k] && t.data[e.menuID] == nil
	case kindTab:
		if e.expandable {
			k := tabKey(e.menuID, e.tab)
			t.expanded[k] = !t.expanded[k]
		}
	}
	return false
}
