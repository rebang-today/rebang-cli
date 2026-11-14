// Package cli 实现 rebang 的命令行入口。
package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/browser"
	"github.com/rebang-today/rebang-cli/internal/config"
	"github.com/rebang-today/rebang-cli/internal/credentials"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/output"
	"github.com/rebang-today/rebang-cli/internal/ui"
)

// App 汇总命令运行所需的依赖，测试时可替换。
type App struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// StdoutTTY 表示标准输出连接到终端。
	StdoutTTY bool
	// StderrTTY 表示标准错误连接到终端，此时可以显示动画。
	StderrTTY bool
	// Color 与 ErrColor 表示标准输出、标准错误可以输出颜色（终端且未设置 NO_COLOR）。
	Color    bool
	ErrColor bool
	// Interactive 表示标准输入与输出均为终端，可以进入交互界面。
	Interactive bool
	TermWidth   int

	Config config.Config
	// OpenURL 打开浏览器。
	OpenURL func(string) error
	// Now 当前时间。
	Now func() time.Time
	// Sleep 等待，设备授权轮询使用。
	Sleep func(context.Context, time.Duration) error
	// RunTUI 进入交互界面。
	RunTUI func(*App) error

	outputFlag  string
	apiBaseFlag string
	format      output.Format
	// noNotice 表示本次命令不附加版本提醒（如 update 自身）。
	noNotice bool
	notice   *noticeCache
}

// NewApp 使用真实终端创建 App。
func NewApp() *App {
	cfg, cfgErr := config.Load()
	a := &App{
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		StdoutTTY:   output.IsTerminal(os.Stdout),
		StderrTTY:   output.IsTerminal(os.Stderr),
		Color:       ui.ColorEnabled(output.IsTerminal(os.Stdout)),
		ErrColor:    ui.ColorEnabled(output.IsTerminal(os.Stderr)),
		Interactive: output.IsTerminal(os.Stdout) && output.IsTerminal(os.Stdin),
		TermWidth:   output.TerminalWidth(os.Stdout),
		Config:      cfg,
		OpenURL:     browser.Open,
		Now:         time.Now,
		Sleep:       sleepCtx,
	}
	if cfgErr != nil {
		// 配置错误不阻止运行，以默认值继续并提示。
		_, _ = io.WriteString(a.Stderr, "警告："+cfgErr.Error()+"\n")
	}
	return a
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// APIBase 返回生效的接口地址。
func (a *App) APIBase() string {
	if a.apiBaseFlag != "" {
		return strings.TrimRight(a.apiBaseFlag, "/")
	}
	return a.Config.API()
}

// Invocation 返回调用方式标记。
func (a *App) Invocation() api.Invocation {
	if a.StdoutTTY {
		return api.InvocationTerminal
	}
	return api.InvocationPipe
}

// Client 返回带当前凭证的 API 客户端。
func (a *App) Client() *api.Client {
	tok, _ := credentials.Token()
	return api.New(a.APIBase(), tok, a.Invocation())
}

// Format 返回本次命令的输出格式。
func (a *App) Format() output.Format {
	if a.format == "" {
		return output.Table
	}
	return a.format
}

// Writer 返回结果输出器。
func (a *App) Writer() output.Writer {
	t := a.Theme()
	w := output.Writer{Out: a.Stdout, Format: a.Format(), Width: a.TermWidth, Theme: &t}
	if w.Format == output.JSON {
		if n := a.Notice(); n != nil {
			w.Notice = n
		}
	}
	return w
}

// Theme 返回标准输出使用的样式。
func (a *App) Theme() ui.Theme { return ui.New(a.Color) }

// ErrTheme 返回标准错误使用的样式。
func (a *App) ErrTheme() ui.Theme { return ui.New(a.ErrColor) }

// errorAsJSON 决定错误输出是否使用 JSON：输出格式为 json 时使用。
func (a *App) errorAsJSON() bool {
	if a.format != "" {
		return a.format == output.JSON
	}
	f, err := output.Resolve(a.outputFlag, a.Config.Output, a.StdoutTTY)
	return err != nil || f == output.JSON
}

// Info 向标准错误输出给人看的提示，不影响标准输出的结构化结果。
func (a *App) Info(format string, args ...any) {
	_, _ = io.WriteString(a.Stderr, sprintf(format, args...))
}

// Execute 运行命令并返回退出码。
func Execute(a *App, args []string) int { return ExecuteContext(context.Background(), a, args) }

// ExecuteContext 在指定上下文中运行命令并返回退出码。
func ExecuteContext(ctx context.Context, a *App, args []string) int {
	root := NewRootCmd(a)
	root.SetContext(ctx)
	root.SetArgs(args)
	root.SetIn(a.Stdin)
	root.SetOut(a.Stdout)
	root.SetErr(a.Stderr)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return errs.ExitOK
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	e := classify(err)
	if a.errorAsJSON() {
		output.WriteError(a.Stderr, e, true)
	} else {
		output.WriteErrorTheme(a.Stderr, e, a.ErrTheme())
	}
	return e.Exit
}

// classify 把 cobra 产生的参数错误归类为退出码 2。
func classify(err error) *errs.Error {
	var e *errs.Error
	if errors.As(err, &e) {
		return e
	}
	msg := err.Error()
	for _, p := range []string{"unknown command", "unknown flag", "unknown shorthand", "flag needs", "invalid argument", "accepts ", "requires at least", "required flag", "if any flags in the group"} {
		if strings.Contains(msg, p) {
			return errs.Usage("%s", msg)
		}
	}
	return errs.From(err)
}

func cobraUsageErr(_ *cobra.Command, err error) error { return errs.Usage("%s", err.Error()) }
