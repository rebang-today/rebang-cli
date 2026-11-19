// Package skills 是随 rebang 发布的 Agent Skill 的唯一来源。
//
// 每个子目录是一个独立的 skill：rebang 是命令行手册，rebang-* 是基于它的应用。
// 程序把它们编译进二进制，由 rebang skill install 写入各 Agent；同一目录也能被
// npx skills add、Claude Code 插件市场直接安装，因此 skill 内容不能依赖安装方式。
package skills

import "embed"

// FS 包含全部 skill 目录。新增应用 skill 时以 rebang- 开头命名即可自动随程序发布。
//
//go:embed rebang rebang-*
var FS embed.FS
