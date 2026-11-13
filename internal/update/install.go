package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"aead.dev/minisign"

	"github.com/rebang-today/rebang-cli/internal/errs"
)

// publicKey 是内置的 minisign 公钥（D-52），发布前写入 minisign.pub。
//
//go:embed minisign.pub
var publicKey string

// PublicKey 返回生效的公钥文本，可在测试中替换。
var PublicKey = func() string { return publicKey }

// VerifySignature 用内置公钥校验 minisign 签名。
func VerifySignature(data, sig []byte) error {
	raw := strings.TrimSpace(PublicKey())
	if raw == "" {
		return errs.New(errs.ExitGeneral, "signature_unavailable", "此版本未内置签名公钥，无法安全升级", "请通过安装脚本或 GitHub Releases 重新安装")
	}
	// 兼容 minisign.pub 文件格式（第一行为注释）。
	lines := strings.Split(raw, "\n")
	keyText := strings.TrimSpace(lines[len(lines)-1])
	var pk minisign.PublicKey
	if err := pk.UnmarshalText([]byte(keyText)); err != nil {
		return errs.New(errs.ExitGeneral, "signature_unavailable", "内置签名公钥格式错误", "").WithCause(err)
	}
	if !minisign.Verify(pk, data, sig) {
		return errs.New(errs.ExitGeneral, "signature_invalid", "安装包签名校验失败，已中止升级", "安装包可能被篡改，请稍后重试或从 GitHub Releases 手动下载")
	}
	return nil
}

// VerifySHA256 校验摘要。
func VerifySHA256(data []byte, want string) error {
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, strings.TrimSpace(want)) {
		return errs.New(errs.ExitGeneral, "checksum_mismatch", "安装包 sha256 校验失败，已中止升级", "稍后重试")
	}
	return nil
}

// binaryName 返回可执行文件名。
func binaryName(goos string) string {
	if goos == "windows" {
		return "rebang.exe"
	}
	return "rebang"
}

// ExtractBinary 从 tar.gz 或 zip 中取出 rebang 可执行文件。
func ExtractBinary(archiveName string, data []byte, goos string) ([]byte, error) {
	want := binaryName(goos)
	if strings.HasSuffix(archiveName, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if path.Base(f.Name) == want && !f.FileInfo().IsDir() {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(io.LimitReader(rc, 256<<20))
			}
		}
		return nil, fmt.Errorf("安装包中没有 %s", want)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("安装包中没有 %s", want)
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && path.Base(h.Name) == want {
			return io.ReadAll(io.LimitReader(tr, 256<<20))
		}
	}
}

// Download 下载并校验指定平台的安装包，返回可执行文件内容。
func Download(ctx context.Context, m *Manifest, platform string) ([]byte, error) {
	asset, ok := m.Platforms[platform]
	if !ok || asset.URL == "" {
		return nil, errs.New(errs.ExitGeneral, errs.CodeNotFound, "新版本没有提供 "+platform+" 平台的安装包", "")
	}
	if asset.SHA256 == "" || asset.SignatureURL == "" {
		return nil, errs.New(errs.ExitGeneral, "checksum_mismatch", "版本信息缺少 sha256 或签名地址，已中止升级", "")
	}
	data, err := get(ctx, asset.URL, 256<<20)
	if err != nil {
		return nil, err
	}
	if err := VerifySHA256(data, asset.SHA256); err != nil {
		return nil, err
	}
	sig, err := get(ctx, asset.SignatureURL, 64<<10)
	if err != nil {
		return nil, err
	}
	if err := VerifySignature(data, sig); err != nil {
		return nil, err
	}
	goos := strings.SplitN(platform, "/", 2)[0]
	bin, err := ExtractBinary(asset.URL, data, goos)
	if err != nil {
		return nil, errs.New(errs.ExitGeneral, errs.CodeGeneral, "解压安装包失败", "").WithCause(err)
	}
	return bin, nil
}

// ManagedBy 判断程序是否由包管理器安装，这类安装应使用包管理器升级。
func ManagedBy(exe string) string {
	p := filepath.ToSlash(exe)
	if strings.Contains(p, "/Cellar/") || strings.Contains(p, "/homebrew/") || strings.Contains(p, "/linuxbrew/") {
		return "homebrew"
	}
	return ""
}

// Replace 原子替换可执行文件。Windows 无法覆盖运行中的程序，先把旧文件改名。
func Replace(exe string, bin []byte) error {
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".rebang-new-*")
	if err != nil {
		return errs.New(errs.ExitGeneral, errs.CodeGeneral, "没有写入 "+dir+" 的权限", "使用有权限的用户重新运行，或重新执行安装脚本").WithCause(err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	tmp.Close()
	if err := os.Chmod(tmpName, 0o755); err != nil {
		os.Remove(tmpName)
		return err
	}
	if runtime.GOOS == "windows" {
		old := exe + ".old"
		_ = os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			os.Remove(tmpName)
			return err
		}
	}
	if err := os.Rename(tmpName, exe); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// CleanupOld 删除 Windows 升级后残留的旧程序。
func CleanupOld(exe string) { _ = os.Remove(exe + ".old") }
