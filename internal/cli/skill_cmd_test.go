package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/update"
)

func TestSkillStatusAllowedBelowMinVersion(t *testing.T) {
	h := newHarness(t)
	setVersion(t, "0.1.0")
	update.SaveState(update.State{MinVersion: "0.2.0"})
	if _, _, code := h.run(false, "skill", "status"); code != 0 {
		t.Fatal(code)
	}
}

func TestSkillCommands(t *testing.T) {
	h := newHarness(t)
	home := os.Getenv("REBANG_HOME")
	out, stderr, code := h.run(false, "skill", "install", "--agent", "claude")
	if code != 0 {
		t.Fatal(stderr)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "rebang", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	out, _, _ = h.run(false, "skill", "status")
	if !strings.Contains(out, `"installed": true`) {
		t.Fatal(out)
	}
	// 升级后同步
	setVersion(t, "9.0.0")
	out, _, code = h.run(false, "skill", "sync")
	// 按 JSON 解析后比较，Windows 路径中的反斜杠在 JSON 里会被转义。
	var synced struct {
		Updated []string `json:"updated"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &synced) != nil || len(synced.Updated) != 1 ||
		!strings.HasSuffix(synced.Updated[0], filepath.Join(".claude", "skills")) {
		t.Fatal(out)
	}
	b, _ := os.ReadFile(filepath.Join(home, ".claude", "skills", "rebang", ".rebang-skill.json"))
	if !strings.Contains(string(b), "9.0.0") {
		t.Fatal("not synced")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "rebang-research", "SKILL.md")); err != nil {
		t.Fatal("应用 skill 应随手册一起安装：", err)
	}
	_, _, code = h.run(false, "skill", "uninstall")
	if code != 0 {
		t.Fatal(code)
	}
	for _, n := range []string{"rebang", "rebang-research"} {
		if _, err := os.Stat(filepath.Join(home, ".claude", "skills", n)); !os.IsNotExist(err) {
			t.Fatal("not removed: " + n)
		}
	}
	_, _, code = h.run(false, "skill", "install", "--agent", "vim")
	if code != errs.ExitUsage {
		t.Fatal(code)
	}
}

func TestDoctor(t *testing.T) {
	h := newHarness(t)
	t.Setenv("REBANG_MANIFEST_URL", "http://127.0.0.1:1/manifest.json")
	out, _, code := h.run(false, "doctor")
	var v struct {
		OK     bool
		Checks []Check
	}
	decode(t, out, &v)
	if code != 0 || !v.OK {
		t.Fatalf("%d %s", code, out)
	}
	names := map[string]string{}
	for _, c := range v.Checks {
		names[c.Name] = c.Status
	}
	if names["接口连通"] != "ok" || names["凭证"] != "warn" || names["版本检查"] != "warn" || names["Agent Skill"] != "warn" {
		t.Fatal(names)
	}
	t.Setenv("REBANG_API_BASE", "http://127.0.0.1:1")
	_, _, code = h.run(false, "doctor")
	if code != errs.ExitGeneral {
		t.Fatal(code)
	}
}
