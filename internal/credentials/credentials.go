// Package credentials 管理 CLI Token（D-11、D-12）。
//
// 凭证只保存在 auth.json 中（0600，原子写入），不接入系统钥匙串；
// 环境变量 REBANG_TOKEN 优先级最高。
package credentials

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/rebang-today/rebang-cli/internal/config"
	"github.com/rebang-today/rebang-cli/internal/fsutil"
)

const (
	// FileName 凭证文件名。
	FileName = "auth.json"
	// FormatVersion 凭证文件格式版本。
	FormatVersion = 1
	// EnvToken 注入凭证的环境变量。
	EnvToken = "REBANG_TOKEN"
)

// File 是 auth.json 的内容。DeviceID 在退出登录后保留，重新登录时服务端据此替换旧 Token。
type File struct {
	Version   int       `json:"version"`
	DeviceID  string    `json:"device_id"`
	Token     string    `json:"token,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// Source 表示 Token 的来源。
type Source string

const (
	SourceNone Source = "none"
	SourceEnv  Source = "env"
	SourceFile Source = "file"
)

// Path 返回凭证文件路径。
func Path() (string, error) { return config.Path(FileName) }

// Load 读取凭证文件；不存在时返回空结构。
func Load() (File, error) {
	p, err := Path()
	if err != nil {
		return File{}, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return File{Version: FormatVersion}, nil
	}
	if err != nil {
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return File{}, fmt.Errorf("凭证文件 %s 已损坏，请删除后重新登录: %w", p, err)
	}
	if f.Version > FormatVersion {
		return File{}, fmt.Errorf("凭证文件版本 %d 高于当前程序支持的版本，请升级 rebang", f.Version)
	}
	f.Version = FormatVersion
	return f, nil
}

// Save 以 0600 权限原子写入凭证文件。
func Save(f File) error {
	p, err := Path()
	if err != nil {
		return err
	}
	f.Version = FormatVersion
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(p, append(b, '\n'), 0o600)
}

// Token 返回当前生效的 Token 及来源：REBANG_TOKEN > auth.json。
func Token() (string, Source) {
	if t := os.Getenv(EnvToken); t != "" {
		return t, SourceEnv
	}
	f, err := Load()
	if err != nil || f.Token == "" {
		return "", SourceNone
	}
	return f.Token, SourceFile
}

// SetToken 保存登录得到的 Token。
func SetToken(token string) error {
	f, err := Load()
	if err != nil {
		f = File{}
	}
	if f.DeviceID == "" {
		f.DeviceID = newDeviceID()
	}
	f.Token = token
	f.CreatedAt = time.Now().UTC()
	return Save(f)
}

// ClearToken 删除本地 Token，保留设备 ID。
func ClearToken() error {
	f, err := Load()
	if err != nil {
		p, perr := Path()
		if perr != nil {
			return perr
		}
		return os.Remove(p)
	}
	if f.Token == "" {
		return nil
	}
	f.Token = ""
	f.CreatedAt = time.Time{}
	return Save(f)
}

// DeviceID 返回持久化的设备 ID，不存在时生成并保存。
func DeviceID() (string, error) {
	f, err := Load()
	if err != nil {
		return "", err
	}
	if f.DeviceID != "" {
		return f.DeviceID, nil
	}
	f.DeviceID = newDeviceID()
	if err := Save(f); err != nil {
		return "", err
	}
	return f.DeviceID, nil
}

func newDeviceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Mask 返回脱敏后的 Token，便于展示。
func Mask(token string) string {
	if len(token) <= 12 {
		return "****"
	}
	return token[:7] + "…" + token[len(token)-4:]
}
