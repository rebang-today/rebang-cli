package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rebang-today/rebang-cli/internal/skill"
	"github.com/rebang-today/rebang-cli/internal/update"
)

// noticeEnv 准备一个有新版本缓存、且 Claude Code 中装有旧版 Skill 的环境。
func noticeEnv(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	setVersion(t, "1.0.0")
	if err := update.SaveState(update.State{LastCheck: time.Now(), LatestVersion: "1.1.0"}); err != nil {
		t.Fatal(err)
	}
	home := os.Getenv("REBANG_HOME")
	ag := skill.KnownAgents(home)[0]
	if err := os.MkdirAll(ag.Home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := skill.Install(ag, "0.9.0", false); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestJSONNotice(t *testing.T) {
	h := noticeEnv(t)
	out, _, code := h.run(false, "menus")
	if code != 0 {
		t.Fatal(code)
	}
	var v struct {
		Menus  []struct{ ID int64 }
		Notice *struct {
			Update *struct{ Current, Latest, Command string }
			Skill  *struct {
				Installed, Expected, Command string
				Dirs                         []string
			}
		} `json:"_notice"`
	}
	decode(t, out, &v)
	if len(v.Menus) != 2 || v.Notice == nil {
		t.Fatalf("应保留原字段并附加 _notice：%s", out)
	}
	u, s := v.Notice.Update, v.Notice.Skill
	if u == nil || u.Latest != "1.1.0" || u.Current != "1.0.0" || u.Command != "rebang update" {
		t.Fatalf("update 提醒不对：%s", out)
	}
	if s == nil || s.Installed != "0.9.0" || s.Expected != "1.0.0" || s.Command != "rebang skill install" ||
		len(s.Dirs) != 1 || !strings.HasSuffix(s.Dirs[0], filepath.Join(".claude", "skills", "rebang")) {
		t.Fatalf("skill 提醒不对：%s", out)
	}
	if !strings.HasPrefix(out, "{\n  \"menus\"") || !strings.HasSuffix(out, "}\n") {
		t.Fatalf("应保持缩进格式与原字段顺序：%s", out)
	}
}

func TestJSONNoticeOnlyWhenNeeded(t *testing.T) {
	h := noticeEnv(t)
	if out, _, _ := h.run(true, "menus"); strings.Contains(out, "_notice") {
		t.Fatal("table 格式不应附加提醒")
	}
	if out, _, _ := h.run(true, "menus", "-o", "json"); !strings.Contains(out, "_notice") {
		t.Fatal("终端中 -o json 也应附加提醒")
	}
	t.Setenv(EnvNoNotice, "1")
	if out, _, _ := h.run(false, "menus"); strings.Contains(out, "_notice") {
		t.Fatal("REBANG_NO_NOTICE=1 应关闭提醒")
	}
}

func TestJSONNoNoticeWhenCurrent(t *testing.T) {
	h := newHarness(t)
	setVersion(t, "1.1.0")
	if err := update.SaveState(update.State{LastCheck: time.Now(), LatestVersion: "1.1.0"}); err != nil {
		t.Fatal(err)
	}
	if out, _, _ := h.run(false, "menus"); strings.Contains(out, "_notice") {
		t.Fatalf("已是最新且未装 Skill 时不应提醒：%s", out)
	}
}

func TestJSONNoticeCheckFailureSilent(t *testing.T) {
	h := newHarness(t)
	setVersion(t, "1.0.0")
	// 缓存过期且清单地址不可用：检查失败应静默，结果照常输出。
	t.Setenv("REBANG_MANIFEST_URL", h.srv.URL+"/missing-manifest.json")
	out, errOut, code := h.run(false, "menus")
	if code != 0 || strings.Contains(out, "_notice") || errOut != "" {
		t.Fatalf("检查失败应静默：code=%d out=%s err=%s", code, out, errOut)
	}
}
