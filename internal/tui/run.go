// Package tui 实现不带参数运行 rebang 时的全屏交互界面（D-20 ~ D-24）。
package tui

import (
	"os"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/buildinfo"
	"github.com/rebang-today/rebang-cli/internal/cli"
	"github.com/rebang-today/rebang-cli/internal/config"
)

// Run 启动交互界面，直到用户退出。
func Run(a *cli.App) error {
	client := a.Client()
	client.Invocation = api.InvocationTUI
	refresh, err := a.Config.Refresh()
	if err != nil {
		return err
	}
	switch a.Config.Theme {
	case "dark":
		lipgloss.SetHasDarkBackground(true)
	case "light":
		lipgloss.SetHasDarkBackground(false)
	}
	opt := Options{
		Source: &apiSource{
			client:      client,
			web:         a.Config.Web(),
			manifestURL: a.Config.Manifest(),
			version:     buildinfo.Version,
			now:         a.Now,
		},
		Version:    buildinfo.Version,
		LoggedIn:   client.Token != "",
		Refresh:    refresh,
		AutoUpdate: a.Config.AutoUpdate,
		Open:       a.OpenURL,
		Copy:       clipboard.WriteAll,
		Now:        a.Now,
	}
	progOpts := []tea.ProgramOption{tea.WithAltScreen(), tea.WithMouseAllMotion()}
	if a.StdoutTTY {
		out := &lockedFile{File: os.Stdout}
		opt.Pointer = out
		progOpts = append(progOpts, tea.WithOutput(out))
		// 无论正常退出、Ctrl+C 还是出错，都把鼠标指针恢复为终端默认。
		defer func() { _, _ = out.Write([]byte(pointerReset)) }()
	}
	// 位置记忆与 update 的 state.json 一样放在配置目录，REBANG_CONFIG_DIR 可整体隔离。
	if p, err := config.Path(stateFileName); err == nil {
		opt.StatePath = p
	}
	p := tea.NewProgram(newModel(opt), progOpts...)
	final, err := p.Run()
	if fm, ok := final.(*model); ok {
		fm.SaveState()
	}
	return err
}
