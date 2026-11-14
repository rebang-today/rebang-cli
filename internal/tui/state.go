package tui

import (
	"encoding/json"
	"os"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rebang-today/rebang-cli/internal/fsutil"
)

// 交互界面的位置记忆（D-21）：上次所在的菜单、每个菜单的来源、每个来源的子榜单。
// 全部按 key 保存，服务端调整菜单或 Tab 顺序后仍能找到；找不到时逐级回退。
// 不保存列表光标、滚动位置与搜索词。

const (
	stateVersion  = 1
	stateFileName = "tui-state.json"
	// saveDelay 是位置变化后写入状态文件前的等待时间，连续切换只写一次。
	saveDelay = time.Second
)

type posState struct {
	Version int `json:"version"`
	// Menu 是上次所在菜单的 ID（订阅为 -1）。
	Menu *int64 `json:"menu,omitempty"`
	// Sources 按菜单 ID 记录上次选中的来源 key（"t:<tab>" 或 "n:<node>"）。
	Sources map[string]string `json:"sources,omitempty"`
	// Subs 按“菜单 ID/来源 key”记录上次手动选中的子榜单 key。
	Subs map[string]string `json:"subs,omitempty"`
	// Preview 是双栏底部的 AI 总结预览是否打开，缺省为打开（D-26）。
	Preview *bool `json:"preview,omitempty"`
}

// loadPosState 读取状态文件；不存在、损坏或版本不认识时返回空记忆。
func loadPosState(path string) posState {
	empty := posState{Version: stateVersion}
	if path == "" {
		return empty
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return empty
	}
	var s posState
	if json.Unmarshal(b, &s) != nil || s.Version != stateVersion {
		return empty
	}
	return s
}

// savePosState 原子写入状态文件（0600，目录 0700）。
func savePosState(path string, s posState) error {
	if path == "" {
		return nil
	}
	s.Version = stateVersion
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, b, 0o600)
}

type saveMsg struct{ seq int }

// snapshot 返回当前要持久化的位置。
func (m *model) snapshot() posState {
	s := posState{Version: stateVersion, Sources: map[string]string{}, Subs: map[string]string{}}
	if mn, ok := m.curMenu(); ok {
		id := mn.ID
		s.Menu = &id
	} else if m.mem.Menu != nil {
		s.Menu = m.mem.Menu
	}
	for k, v := range m.srcMem {
		s.Sources[strconv.FormatInt(k, 10)] = v
	}
	for k, v := range m.subMem {
		s.Subs[k] = v
	}
	preview := !m.previewOff
	s.Preview = &preview
	return s
}

// markDirty 在位置变化后安排一次延迟写入。
func (m *model) markDirty() tea.Cmd {
	if m.opt.StatePath == "" {
		return nil
	}
	m.saveSeq++
	seq := m.saveSeq
	return tea.Tick(saveDelay, func(time.Time) tea.Msg { return saveMsg{seq} })
}

// SaveState 立即写入当前位置，退出时调用。读写失败不影响使用，直接忽略。
func (m *model) SaveState() {
	if m.opt.StatePath == "" {
		return
	}
	if len(m.menus) == 0 {
		// 菜单还没加载：只更新预览开关，其余沿用启动时读到的记忆。
		s := m.mem
		preview := !m.previewOff
		s.Preview = &preview
		_ = savePosState(m.opt.StatePath, s)
		return
	}
	m.savePos()
	_ = savePosState(m.opt.StatePath, m.snapshot())
}
