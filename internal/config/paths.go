// Package config 负责配置文件与本地目录（D-40）。
package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// Dir 返回配置目录：macOS/Linux 为 ~/.config/rebang，Windows 为 %AppData%\rebang。
// 环境变量 REBANG_CONFIG_DIR 可覆盖，主要用于测试与多账号隔离。
func Dir() (string, error) {
	if d := os.Getenv("REBANG_CONFIG_DIR"); d != "" {
		return d, nil
	}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("AppData"); appData != "" {
			return filepath.Join(appData, "rebang"), nil
		}
		d, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(d, "rebang"), nil
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "rebang"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "rebang"), nil
}

// Path 返回配置目录下的文件路径。
func Path(name string) (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, name), nil
}
