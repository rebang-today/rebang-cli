package update

import (
	"fmt"
	"strings"
	"testing"

	"aead.dev/minisign"
)

// TestEmbeddedPublicKey 确认内置的是正式发布公钥：minisign.pub 格式、能解析、key ID 正确。
func TestEmbeddedPublicKey(t *testing.T) {
	lines := strings.Split(strings.TrimSpace(publicKey), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "untrusted comment: ") {
		t.Fatalf("minisign.pub 应为两行标准格式：%q", publicKey)
	}
	var pk minisign.PublicKey
	if err := pk.UnmarshalText([]byte(strings.TrimSpace(lines[1]))); err != nil {
		t.Fatal(err)
	}
	if id := fmt.Sprintf("%016X", pk.ID()); id != "05B5851321C39815" || !strings.HasSuffix(lines[0], id) {
		t.Fatalf("key ID %s 与注释不一致或不是正式公钥：%s", id, lines[0])
	}
	// 内置公钥走 VerifySignature 的解析路径：格式正确，只是签名不匹配。
	err := VerifySignature([]byte("x"), []byte("not a signature"))
	if err == nil || strings.Contains(err.Error(), "格式错误") || strings.Contains(err.Error(), "未内置") {
		t.Fatalf("内置公钥应能被解析，得到签名不匹配的错误：%v", err)
	}
}
