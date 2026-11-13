// Package skill 管理随程序发布的 Agent Skill（D-60 ~ D-62）。
package skill

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rebang-today/rebang-cli/internal/fsutil"
)

//go:embed files
var files embed.FS

// Name 是 skill 目录名。
const Name = "rebang"

// markerFile 记录由 rebang 安装的 skill 及其版本，避免误删用户自己的同名目录。
const markerFile = ".rebang-skill.json"

type marker struct {
	Version string `json:"version"`
}

// Agent 是一个支持 skill 的 Agent 及其 skill 根目录。
type Agent struct {
	Name string `json:"agent"`
	// Home 是该 Agent 的配置目录，存在时视为已安装该 Agent。
	Home string `json:"-"`
	// Root 是 skill 根目录，skill 安装到 Root/rebang。
	Root string `json:"skills_dir"`
}

// Dir 返回 skill 安装目录。
func (a Agent) Dir() string { return filepath.Join(a.Root, Name) }

// KnownAgents 返回支持的 Agent 列表。
func KnownAgents(home string) []Agent {
	return []Agent{
		{Name: "claude", Home: filepath.Join(home, ".claude"), Root: filepath.Join(home, ".claude", "skills")},
		{Name: "codex", Home: filepath.Join(home, ".codex"), Root: filepath.Join(home, ".codex", "skills")},
		{Name: "agents", Home: filepath.Join(home, ".agents"), Root: filepath.Join(home, ".agents", "skills")},
	}
}

// Select 根据名称选择 Agent；names 为空时选择本机已存在配置目录的 Agent，都没有时选择 claude。
func Select(home string, names []string) ([]Agent, error) {
	all := KnownAgents(home)
	if len(names) == 0 {
		var out []Agent
		for _, a := range all {
			if st, err := os.Stat(a.Home); err == nil && st.IsDir() {
				out = append(out, a)
			}
		}
		if len(out) == 0 {
			out = all[:1]
		}
		return out, nil
	}
	var out []Agent
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "all" {
			return all, nil
		}
		found := false
		for _, a := range all {
			if a.Name == n {
				out = append(out, a)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("不支持的 Agent %q，可选 claude、codex、agents、all", n)
		}
	}
	return out, nil
}

// Render 返回替换版本号后的全部文件，键为相对路径（使用 /）。
func Render(version string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := fs.WalkDir(files, "files", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		out[strings.TrimPrefix(p, "files/")] = []byte(strings.ReplaceAll(string(b), "{{VERSION}}", version))
		return nil
	})
	return out, err
}

// ErrForeign 表示目标目录存在但不是 rebang 安装的。
var ErrForeign = errors.New("目标目录已存在且不是由 rebang 安装的")

// Install 把 skill 写入 dir。force 为 true 时覆盖非 rebang 安装的目录。
func Install(dir, version string, force bool) error {
	if _, err := os.Stat(dir); err == nil {
		if _, ok := readMarker(dir); !ok && !force {
			return ErrForeign
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	rendered, err := Render(version)
	if err != nil {
		return err
	}
	for rel, b := range rendered {
		if err := fsutil.WriteFileAtomic(filepath.Join(dir, filepath.FromSlash(rel)), b, 0o644); err != nil {
			return err
		}
	}
	mb, _ := json.Marshal(marker{Version: version})
	return fsutil.WriteFileAtomic(filepath.Join(dir, markerFile), mb, 0o644)
}

// Uninstall 删除由 rebang 安装的 skill，返回是否删除。
func Uninstall(dir string) (bool, error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if _, ok := readMarker(dir); !ok {
		return false, ErrForeign
	}
	return true, os.RemoveAll(dir)
}

// Status 是某个 Agent 下 skill 的安装状态。
type Status struct {
	Agent     string `json:"agent"`
	Dir       string `json:"dir"`
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
	UpToDate  bool   `json:"up_to_date"`
	Foreign   bool   `json:"foreign,omitempty"`
}

// StatusOf 查询安装状态。
func StatusOf(a Agent, current string) Status {
	s := Status{Agent: a.Name, Dir: a.Dir()}
	if _, err := os.Stat(a.Dir()); err != nil {
		return s
	}
	m, ok := readMarker(a.Dir())
	if !ok {
		s.Foreign = true
		return s
	}
	s.Installed = true
	s.Version = m.Version
	s.UpToDate = m.Version == current
	return s
}

// Sync 把所有已安装但版本不一致的 skill 更新到当前版本，返回更新过的目录。
func Sync(home, current string) ([]string, error) {
	var updated []string
	var errsOut []string
	for _, a := range KnownAgents(home) {
		st := StatusOf(a, current)
		if !st.Installed || st.UpToDate {
			continue
		}
		if err := Install(a.Dir(), current, false); err != nil {
			errsOut = append(errsOut, a.Dir()+": "+err.Error())
			continue
		}
		updated = append(updated, a.Dir())
	}
	sort.Strings(updated)
	if len(errsOut) > 0 {
		return updated, errors.New(strings.Join(errsOut, "; "))
	}
	return updated, nil
}

func readMarker(dir string) (marker, bool) {
	b, err := os.ReadFile(filepath.Join(dir, markerFile))
	if err != nil {
		return marker{}, false
	}
	var m marker
	if json.Unmarshal(b, &m) != nil {
		return marker{}, false
	}
	return m, true
}
