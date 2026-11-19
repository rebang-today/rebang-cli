package cli

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/rebang-today/rebang-cli/skills"
)

// 手册 skill 是 Agent 了解命令的唯一途径：新增命令或参数时必须同步写进 skills/rebang，
// 否则 Agent 不知道它的存在。这里逐个核对命令路径与参数名是否出现在手册中。
func TestSkillManualCoversCommands(t *testing.T) {
	var doc strings.Builder
	fs.WalkDir(skills.FS, "rebang", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := skills.FS.ReadFile(p)
			doc.Write(b)
		}
		return nil
	})
	manual := doc.String()

	// 交互界面、补全与帮助不是给 Agent 用的。
	skip := map[string]bool{"help": true, "completion": true}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.Hidden || skip[sub.Name()] {
				continue
			}
			path := sub.CommandPath()
			if len(sub.Commands()) == 0 && !strings.Contains(manual, path) {
				t.Errorf("手册中没有命令 %q", path)
			}
			sub.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
				if f.Hidden || f.Name == "help" {
					return
				}
				if !strings.Contains(manual, "--"+f.Name) {
					t.Errorf("手册中没有 %s 的参数 --%s", path, f.Name)
				}
			})
			walk(sub)
		}
	}
	walk(NewRootCmd(&App{}))
}
