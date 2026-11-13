// Command rebang 是今日热榜（rebang.today）的官方命令行客户端。
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/rebang-today/rebang-cli/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	app := cli.NewApp()
	app.RunTUI = runTUI
	code := cli.ExecuteContext(ctx, app, os.Args[1:])
	stop()
	os.Exit(code)
}
