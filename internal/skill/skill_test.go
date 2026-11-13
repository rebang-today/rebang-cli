package skill

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderSubstitutesVersion(t *testing.T) {
	r, err := Render("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SKILL.md", "references/research.md"} {
		b, ok := r[name]
		if !ok {
			t.Fatalf("missing %s", name)
		}
		if strings.Contains(string(b), "{{VERSION}}") || !strings.Contains(string(b), "1.2.3") {
			t.Fatalf("%s not rendered", name)
		}
	}
	if !strings.HasPrefix(string(r["SKILL.md"]), "---\nname: rebang\n") {
		t.Fatal("frontmatter")
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
	if err := Install(a.Dir(), "0.1.0", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.Dir(), "references", "research.md")); err != nil {
		t.Fatal(err)
	}
	st := StatusOf(a, "0.2.0")
	if !st.Installed || st.UpToDate || st.Version != "0.1.0" {
		t.Fatalf("%+v", st)
	}
	updated, err := Sync(home, "0.2.0")
	if err != nil || len(updated) != 1 {
		t.Fatalf("%v %v", updated, err)
	}
	if !StatusOf(a, "0.2.0").UpToDate {
		t.Fatal("not synced")
	}
	b, _ := os.ReadFile(filepath.Join(a.Dir(), "SKILL.md"))
	if !strings.Contains(string(b), "0.2.0") {
		t.Fatal("content not updated")
	}
	ok, err := Uninstall(a.Dir())
	if err != nil || !ok {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.Dir()); !os.IsNotExist(err) {
		t.Fatal("not removed")
	}
}

func TestForeignDirectoryProtected(t *testing.T) {
	home := t.TempDir()
	a := KnownAgents(home)[0]
	os.MkdirAll(a.Dir(), 0o755)
	os.WriteFile(filepath.Join(a.Dir(), "SKILL.md"), []byte("mine"), 0o644)
	if err := Install(a.Dir(), "0.1.0", false); !errors.Is(err, ErrForeign) {
		t.Fatalf("got %v", err)
	}
	if _, err := Uninstall(a.Dir()); !errors.Is(err, ErrForeign) {
		t.Fatalf("got %v", err)
	}
	if st := StatusOf(a, "0.1.0"); !st.Foreign {
		t.Fatal("should be foreign")
	}
	if err := Install(a.Dir(), "0.1.0", true); err != nil {
		t.Fatal(err)
	}
}

func TestSelectByName(t *testing.T) {
	if a, _ := Select("/h", []string{"all"}); len(a) != 3 {
		t.Fatal("all")
	}
	if _, err := Select("/h", []string{"vim"}); err == nil {
		t.Fatal("unknown")
	}
}
