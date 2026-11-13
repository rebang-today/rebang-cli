// Package buildinfo 保存编译时注入的版本信息。
package buildinfo

import (
	"fmt"
	"runtime"
)

// 以下变量由 goreleaser 通过 -ldflags -X 注入。
var (
	Version = "0.0.0-dev"
	Commit  = "none"
	Date    = "unknown"
)

// Platform 返回 <os>/<arch>，例如 darwin/arm64。
func Platform() string { return runtime.GOOS + "/" + runtime.GOARCH }

// UserAgent 返回 D-33 约定的 User-Agent。
func UserAgent() string {
	return fmt.Sprintf("rebang-cli/%s (%s)", Version, Platform())
}
