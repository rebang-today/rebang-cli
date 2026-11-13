// Command rebang 是今日热榜（rebang.today）的官方命令行客户端。
package main

import (
	"fmt"

	"github.com/rebang-today/rebang-cli/internal/buildinfo"
)

func main() {
	fmt.Println("rebang", buildinfo.Version)
}
