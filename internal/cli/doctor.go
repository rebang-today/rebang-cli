package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/buildinfo"
	"github.com/rebang-today/rebang-cli/internal/config"
	"github.com/rebang-today/rebang-cli/internal/credentials"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/output"
)

// Check 是一项诊断结果。
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok、warn、fail
	Detail string `json:"detail"`
}

func newDoctorCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "环境诊断",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			checks := RunDoctor(cmd.Context(), a)
			rows := [][]string{}
			failed := false
			for _, c := range checks {
				mark := map[string]string{"ok": "✓", "warn": "!", "fail": "✗"}[c.Status]
				rows = append(rows, []string{mark, c.Name, c.Detail})
				failed = failed || c.Status == "fail"
			}
			if err := a.Writer().Write(output.Result{
				Data:    map[string]any{"checks": checks, "ok": !failed},
				Columns: []output.Column{{Title: "", Kind: output.KindStatus}, {Title: "检查项"}, {Title: "结果", Shrink: true}},
				Rows:    rows,
			}); err != nil {
				return err
			}
			if failed {
				return errs.New(errs.ExitGeneral, "doctor_failed", "部分检查未通过", "按上面的结果逐项处理")
			}
			return nil
		},
	}
}

// doctorChecks 是其他模块注册的诊断项（升级、skill）。
var doctorChecks []func(context.Context, *App) []Check

// RunDoctor 执行各项诊断。
func RunDoctor(ctx context.Context, a *App) []Check {
	var cs []Check
	add := func(name, status, detail string) { cs = append(cs, Check{name, status, detail}) }

	add("版本", "ok", fmt.Sprintf("rebang %s (%s)", buildinfo.Version, buildinfo.Platform()))

	dir, _ := config.Dir()
	if _, err := config.Load(); err != nil {
		add("配置文件", "fail", err.Error())
	} else {
		add("配置文件", "ok", filepath.Join(dir, config.FileName))
	}

	tok, src := credentials.Token()
	switch src {
	case credentials.SourceEnv:
		add("凭证", "ok", "来自环境变量 "+credentials.EnvToken)
	case credentials.SourceFile:
		p, _ := credentials.Path()
		detail := p
		if runtime.GOOS != "windows" {
			if st, err := os.Stat(p); err == nil && st.Mode().Perm()&0o077 != 0 {
				add("凭证", "warn", fmt.Sprintf("%s 权限为 %v，建议 chmod 600", p, st.Mode().Perm()))
				break
			}
		}
		add("凭证", "ok", detail)
	default:
		if _, err := credentials.Load(); err != nil {
			add("凭证", "fail", err.Error())
		} else {
			add("凭证", "warn", "未登录，运行 rebang auth login")
		}
	}

	c := api.New(a.APIBase(), tok, a.Invocation())
	c.HTTP.Timeout = 10 * time.Second
	start := time.Now()
	if _, err := c.Menus(ctx); err != nil {
		add("接口连通", "fail", a.APIBase()+"："+errs.From(api.MapError(err)).Error())
	} else {
		add("接口连通", "ok", fmt.Sprintf("%s（%d ms）", a.APIBase(), time.Since(start).Milliseconds()))
		if tok != "" {
			if acct, err := c.Account(ctx); err != nil {
				e := errs.From(api.MapError(err))
				status := "warn"
				if e.Exit == errs.ExitAuth {
					status = "fail"
				}
				add("登录状态", status, e.Message)
			} else {
				add("登录状态", "ok", toAccountView(acct, a.Now(), a.Config.Web()).Email)
			}
		}
	}

	for _, extra := range doctorChecks {
		cs = append(cs, extra(ctx, a)...)
	}
	return cs
}
