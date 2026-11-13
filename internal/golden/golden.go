// Package golden 提供快照测试：比较输出与 testdata/golden 下的文件。
// 设置环境变量 UPDATE_GOLDEN=1 运行测试即可重新生成快照。
package golden

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Assert 比较 got 与 testdata/golden/<name>.golden。
func Assert(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", "golden", name+".golden")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("读取快照 %s 失败（UPDATE_GOLDEN=1 生成）：%v", p, err)
	}
	if string(want) != got {
		t.Errorf("%s 与快照不一致（UPDATE_GOLDEN=1 更新）\n--- 期望\n%s\n--- 实际\n%s", p, want, got)
	}
}

// StripANSI 去掉 ANSI 控制序列，便于检查带颜色输出的文字内容。
func StripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
