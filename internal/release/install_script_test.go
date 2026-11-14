package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallScriptRunsRebangWithDumbTerm 确保 install.sh 运行新装的 rebang 时都带 TERM=dumb。
// 程序启动时（bubbletea 的 init）会在标准输出是终端时发送 ESC]11;? 与 ESC[6n 查询终端，
// curl | sh 时终端的应答可能残留到用户的 shell 提示符；TERM=dumb 时 termenv 不发送查询。
func TestInstallScriptRunsRebangWithDumbTerm(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	runs := 0
	for i, line := range strings.Split(string(b), "\n") {
		l := strings.TrimSpace(line)
		idx := strings.Index(l, `"$INSTALL_DIR/rebang" `)
		if idx < 0 || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "say ") || strings.HasPrefix(l, "mv ") {
			continue
		}
		runs++
		if !strings.HasSuffix(strings.TrimSpace(l[:idx]), "TERM=dumb") {
			t.Errorf("install.sh 第 %d 行运行 rebang 时缺少 TERM=dumb：%s", i+1, l)
		}
	}
	if runs < 2 {
		t.Fatalf("应至少检查到 version 与 skill install 两处调用，实际 %d 处", runs)
	}
}
