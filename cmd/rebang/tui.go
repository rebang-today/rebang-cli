package main

import "github.com/rebang-today/rebang-cli/internal/cli"

// runTUI 由交互界面包提供，见 internal/tui。
var runTUI func(*cli.App) error
