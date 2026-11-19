package skill

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNamesAndFrontmatter(t *testing.T) {
	names := Names()
	if len(names) < 2 || names[0] != Primary {
		t.Fatalf("手册 skill 应排在最前且至少有一个应用 skill：%v", names)
	}
	for _, n := range names {
		files, err := Files(n)
		if err != nil {
			t.Fatal(err)
		}
		b, ok := files["SKILL.md"]
		if !ok {
			t.Fatalf("%s 缺少 SKILL.md", n)
		}
		// 目录名必须与 frontmatter 中的 name 一致，否则 npx skills 等工具无法识别。
		if !strings.HasPrefix(string(b), "---\nname: "+n+"\n") {
			t.Fatalf("%s frontmatter 的 name 不对", n)
		}
		if strings.Contains(string(b), "{{") {
			t.Fatalf("%s 含有未替换的占位符；skill 也会被 npx skills 原样安装", n)
		}
	}
}

func TestInstallStatusSyncUninstall(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	agents, _ := Select(home, nil)
	if len(agents) != 1 || agents[0].Name != "codex" {
		t.Fatalf("auto select %+v", agents)
	}
	a := agents[0]
	if err := Install(a, "0.1.0", false); err != nil {
		t.Fatal(err)
	}
	for _, n := range Names() {
		if _, err := os.Stat(filepath.Join(a.Root, n, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
	st := StatusOf(a, "0.2.0")
	if !st.Installed || st.UpToDate || st.Version != "0.1.0" || len(st.Skills) != len(Names()) {
		t.Fatalf("%+v", st)
	}
	updated, err := Sync(home, "0.2.0")
	if err != nil || len(updated) != 1 {
		t.Fatalf("%v %v", updated, err)
	}
	if !StatusOf(a, "0.2.0").UpToDate {
		t.Fatal("not synced")
	}
	ok, err := Uninstall(a)
	if err != nil || !ok {
		t.Fatal(err)
	}
	for _, n := range Names() {
		if _, err := os.Stat(filepath.Join(a.Root, n)); !os.IsNotExist(err) {
			t.Fatal("not removed: " + n)
		}
	}
}

// 旧版本只装了手册 skill，升级后新增的应用 skill 也要被同步进来。
func TestSyncInstallsNewSkills(t *testing.T) {
	home := t.TempDir()
	a := KnownAgents(home)[0]
	if err := Install(a, "0.1.0", false); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(a.Root, "rebang-research"))
	if StatusOf(a, "0.1.0").UpToDate {
		t.Fatal("缺少应用 skill 时不应视为最新")
	}
	if _, err := Sync(home, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.Root, "rebang-research", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestForeignDirectoryProtected(t *testing.T) {
	home := t.TempDir()
	a := KnownAgents(home)[0]
	os.MkdirAll(a.Dir(), 0o755)
	os.WriteFile(filepath.Join(a.Dir(), "SKILL.md"), []byte("mine"), 0o644)
	err := Install(a, "0.1.0", false)
	var fe *ForeignError
	if !errors.Is(err, ErrForeign) || !errors.As(err, &fe) || fe.Dir != a.Dir() {
		t.Fatalf("got %v", err)
	}
	// 冲突时一个 skill 都不写。
	if _, err := os.Stat(filepath.Join(a.Root, "rebang-research")); !os.IsNotExist(err) {
		t.Fatal("冲突时不应写入其他 skill")
	}
	if _, err := Uninstall(a); !errors.Is(err, ErrForeign) {
		t.Fatalf("got %v", err)
	}
	if st := StatusOf(a, "0.1.0"); !st.Foreign {
		t.Fatal("should be foreign")
	}
	b, _ := os.ReadFile(filepath.Join(a.Dir(), "SKILL.md"))
	if string(b) != "mine" {
		t.Fatal("用户自己的目录被改动")
	}
	if err := Install(a, "0.1.0", true); err != nil {
		t.Fatal(err)
	}
}

func TestSelectByName(t *testing.T) {
	if a, _ := Select("/h", []string{"all"}); len(a) != len(KnownAgents("/h")) {
		t.Fatal("all")
	}
	if a, _ := Select("/h", []string{"cursor"}); len(a) != 1 || a[0].Root != filepath.Join("/h", ".cursor", "skills") {
		t.Fatal("cursor")
	}
	if _, err := Select("/h", []string{"vim"}); err == nil {
		t.Fatal("unknown")
	}
}
