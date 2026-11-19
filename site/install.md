# 今日热榜 rebang CLI 安装指南

你正在帮用户安装今日热榜（rebang.today）的命令行 `rebang` 和它的 Agent Skill。装好后，用户可以直接用自然语言问你各平台的热榜、热点和 AI 总结。按下面的步骤做，每步看结果再继续。

## 1. 检查是否已安装

```bash
rebang version
```

能输出版本号就说明已安装，跳到第 3 步。

## 2. 安装程序与 Skill

按用户的系统选一条运行：

macOS / Linux：

```bash
curl -fsSL https://cli.rebang.today/install.sh | sh -s -- --with-skill
```

Windows（PowerShell）：

```powershell
$env:REBANG_INSTALL_SKILL=1; irm https://cli.rebang.today/install.ps1 | iex
```

- 安装脚本会校验 sha256，程序装在 `~/.local/bin/rebang`（Windows 为 `%LOCALAPPDATA%\Programs\rebang\rebang.exe`）。
- 如果之后提示找不到 `rebang`，本次会话里用完整路径运行；安装脚本会打印把目录加入 PATH 的命令，转告用户即可，不要自行修改用户的 shell 配置文件。

## 3. 确认 Skill 已装到你这里

```bash
rebang skill status -o json
```

找到你所在 Agent 那一项（`claude`、`codex`、`agents`、`cursor`、`gemini`）：

- `installed` 为 true：完成。
- 没有安装：运行 `rebang skill install --agent <名称>`。
- `foreign` 为 true：用户已经用别的方式（如 `npx skills`）装过同名 skill，不要覆盖。
- 你所在的 Agent 不在列表中：运行 `rebang skill install --dir <你的 skill 根目录>`，或 `npx skills add rebang-today/rebang-cli -g`。

## 4. 验证

```bash
rebang list --tab weibo -n 5 -o json
```

这一步不需要登录。返回 `items` 即可。

## 5. 告诉用户

用一两句话告诉用户安装完成，并给几个可以直接问你的例子：

- “今天微博和知乎都在聊什么？”
- “最近一周大家怎么看某个公司或话题？”
- “这条热搜是怎么回事？”

如果你需要重新加载才能使用新的 skill（例如要开一个新会话），一并说明。

## 注意

- 不要让用户登录。热榜、搜索、AI 总结都不需要登录；以后用到我的关注、内容监控时，skill 会告诉你如何按需登录。
- 不要修改系统设置，不要用 sudo。
- 安装失败时把错误原样告诉用户，并建议运行 `rebang doctor`。
