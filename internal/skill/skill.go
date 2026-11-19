// Package skill 管理随程序发布的 Agent Skill（D-60 ~ D-62）。
//
// skill 内容的唯一来源是仓库根目录的 skills/，每个子目录是一个 skill。
// 安装时把全部 skill 作为一组写入 Agent 的 skill 根目录，组内版本保持一致。
package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rebang-today/rebang-cli/internal/fsutil"
	"github.com/rebang-today/rebang-cli/skills"
)

// Primary 是命令行手册 skill 的目录名，其余应用 skill 依赖它；安装状态以它为准。
const Primary = "rebang"

// markerFile 记录由 rebang 安装的 skill 及其版本，避免误删用户自己的同名目录，
// 也让通过 npx skills 等方式安装的同名目录不被程序接管。
const markerFile = ".rebang-skill.json"

type marker struct {
	Version string `json:"version"`
}

// Agent 是一个支持 skill 的 Agent 及其 skill 根目录。
type Agent struct {
	Name string `json:"agent"`
	// Home 是该 Agent 的配置目录，存在时视为已安装该 Agent。
	Home string `json:"-"`
	// Root 是 skill 根目录，每个 skill 安装到 Root/<名称>。
	Root string `json:"skills_dir"`
}

// Dir 返回手册 skill 的安装目录。
func (a Agent) Dir() string { return filepath.Join(a.Root, Primary) }

// KnownAgents 返回支持的 Agent 列表。
func KnownAgents(home string) []Agent {
	return []Agent{
		{Name: "claude", Home: filepath.Join(home, ".claude"), Root: filepath.Join(home, ".claude", "skills")},
		{Name: "codex", Home: filepath.Join(home, ".codex"), Root: filepath.Join(home, ".codex", "skills")},
		{Name: "agents", Home: filepath.Join(home, ".agents"), Root: filepath.Join(home, ".agents", "skills")},
		{Name: "cursor", Home: filepath.Join(home, ".cursor"), Root: filepath.Join(home, ".cursor", "skills")},
		{Name: "gemini", Home: filepath.Join(home, ".gemini"), Root: filepath.Join(home, ".gemini", "skills")},
	}
}

// AgentNames 返回可用于 --agent 的名称。
func AgentNames() []string {
	var out []string
	for _, a := range KnownAgents("") {
		out = append(out, a.Name)
	}
	return out
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
			return nil, fmt.Errorf("不支持的 Agent %q，可选 %s、all", n, strings.Join(AgentNames(), "、"))
		}
	}
	return out, nil
}

// Names 返回随程序发布的全部 skill 名称，手册 skill 排在最前。
func Names() []string {
	entries, _ := fs.ReadDir(skills.FS, ".")
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool {
		if (names[i] == Primary) != (names[j] == Primary) {
			return names[i] == Primary
		}
		return names[i] < names[j]
	})
	return names
}

// Files 返回某个 skill 的全部文件，键为相对路径（使用 /）。
func Files(name string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := fs.WalkDir(skills.FS, name, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := skills.FS.ReadFile(p)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, name+"/")
		out[path.Clean(rel)] = b
		return nil
	})
	return out, err
}

// ErrForeign 表示目标目录存在但不是 rebang 安装的。
var ErrForeign = errors.New("目标目录已存在且不是由 rebang 安装的")

// ForeignError 指出哪个目录不是由 rebang 安装的。
type ForeignError struct{ Dir string }

func (e *ForeignError) Error() string { return e.Dir + " " + ErrForeign.Error() }

// Is 让 errors.Is(err, ErrForeign) 成立。
func (e *ForeignError) Is(target error) bool { return target == ErrForeign }

// Install 把全部 skill 写入 a.Root。force 为 true 时覆盖非 rebang 安装的同名目录。
// 先检查全部目标再写入，避免装了一半才发现冲突。
func Install(a Agent, version string, force bool) error {
	names := Names()
	if !force {
		for _, n := range names {
			dir := filepath.Join(a.Root, n)
			if _, err := os.Stat(dir); err == nil {
				if _, ok := readMarker(dir); !ok {
					return &ForeignError{Dir: dir}
				}
			}
		}
	}
	for _, n := range names {
		if err := installOne(filepath.Join(a.Root, n), n, version); err != nil {
			return err
		}
	}
	return nil
}

func installOne(dir, name, version string) error {
	files, err := Files(name)
	if err != nil {
		return err
	}
	// 整个目录重建，保证旧版本删除的文件不会残留。
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	for rel, b := range files {
		if err := fsutil.WriteFileAtomic(filepath.Join(dir, filepath.FromSlash(rel)), b, 0o644); err != nil {
			return err
		}
	}
	mb, _ := json.Marshal(marker{Version: version})
	return fsutil.WriteFileAtomic(filepath.Join(dir, markerFile), mb, 0o644)
}

// Uninstall 删除 a.Root 下由 rebang 安装的全部 skill，返回是否删除了手册 skill。
// 非 rebang 安装的同名目录保留；手册 skill 是这种目录时返回 ErrForeign。
func Uninstall(a Agent) (bool, error) {
	removed := false
	var foreign error
	for _, n := range Names() {
		dir := filepath.Join(a.Root, n)
		if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if _, ok := readMarker(dir); !ok {
			if n == Primary {
				foreign = &ForeignError{Dir: dir}
			}
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			return removed, err
		}
		if n == Primary {
			removed = true
		}
	}
	return removed, foreign
}

// Status 是某个 Agent 下 skill 的安装状态。
type Status struct {
	Agent     string `json:"agent"`
	Dir       string `json:"dir"`
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
	// Skills 是由 rebang 安装的 skill 名称。
	Skills   []string `json:"skills"`
	UpToDate bool     `json:"up_to_date"`
	Foreign  bool     `json:"foreign,omitempty"`
}

// StatusOf 查询安装状态。手册 skill 由 rebang 安装，且全部 skill 都是当前版本时才算最新；
// 升级后新增的应用 skill 尚未安装时同样视为需要同步。
func StatusOf(a Agent, current string) Status {
	s := Status{Agent: a.Name, Dir: a.Dir(), Skills: []string{}}
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
	s.UpToDate = true
	for _, n := range Names() {
		mm, ok := readMarker(filepath.Join(a.Root, n))
		if ok {
			s.Skills = append(s.Skills, n)
		}
		if !ok || mm.Version != current {
			s.UpToDate = false
		}
	}
	return s
}

// Sync 把所有已安装但版本不一致的 skill 更新到当前版本，返回更新过的 skill 根目录。
func Sync(home, current string) ([]string, error) {
	var updated []string
	var errsOut []string
	for _, a := range KnownAgents(home) {
		st := StatusOf(a, current)
		if !st.Installed || st.UpToDate {
			continue
		}
		if err := Install(a, current, false); err != nil {
			errsOut = append(errsOut, a.Root+": "+err.Error())
			continue
		}
		updated = append(updated, a.Root)
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
