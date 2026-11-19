# rebang Agent Skills

这里是随 rebang 发布的 Agent Skill 的唯一来源。分两层：

| 目录 | 层 | 说明 |
| --- | --- | --- |
| `rebang/` | 命令行手册 | 每个命令能做什么、参数、JSON 字段、退出码、按需登录。`SKILL.md` 是入口，细节按主题放在 `references/`，Agent 按需读取 |
| `rebang-research/` | 应用 | 话题研究：多次搜索、筛选归并、读 AI 总结，输出带来源的简报 |

应用 skill 只描述“怎么把命令组合成一件事、输出成什么样”，命令细节一律引用 `rebang` 手册，不重复抄写。

## 安装

```bash
rebang skill install                                  # 随程序安装，rebang update 后自动同步
npx skills add rebang-today/rebang-cli -g             # 或者用 skills 工具安装到任意 Agent
```

Claude Code 也可以通过插件市场安装：`/plugin marketplace add rebang-today/rebang-cli`，再 `/plugin install rebang@rebang-today`。

## 新增应用 skill

1. 新建 `rebang-<名称>/SKILL.md`，frontmatter 的 `name` 与目录名一致；`description` 写用户会怎么说，而不是用到哪些命令。
2. 开头说明依赖 `rebang` skill；写清步骤、限制处理与输出格式。
3. 在 `rebang/SKILL.md` 的“基于本手册的应用”表中登记。
4. 运行 `make check`：测试会检查 frontmatter、占位符，以及手册是否覆盖了全部命令与参数。

以 `rebang-` 开头的目录会自动编译进程序，并作为一组安装。

规划中的应用：

- `rebang-briefing`：每日或每周热点简报，可按用户关注的领域定制。
- `rebang-monitor-digest`：定时汇总内容监控的新命中。
