// Package tui 实现不带参数运行 rebang 时的全屏交互界面（D-20 ~ D-24）。
package tui

import (
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/buildinfo"
	"github.com/rebang-today/rebang-cli/internal/cli"
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
	p := tea.NewProgram(newModel(opt), tea.WithAltScreen(), tea.WithMouseAllMotion())
	_, err = p.Run()
	return err
}
