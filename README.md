# rebang

今日热榜（[rebang.today](https://rebang.today)）官方命令行客户端。

- 不带参数运行进入全屏交互界面：分类 → 榜单 → 子榜单浏览、搜索、订阅，回车用浏览器打开。
- 带子命令运行输出结构化结果，适合脚本与 AI Agent 调用；输出到终端时默认表格，被管道读取时默认 JSON。
- 内置 Agent Skill，一条命令装进 Claude Code、Codex 等 Agent。

## 安装

macOS / Linux：

```bash
curl -fsSL https://cli.rebang.today/install.sh | sh
```

Windows（PowerShell）：

```powershell
irm https://cli.rebang.today/install.ps1 | iex
```

Homebrew（macOS）：

```bash
brew install rebang-today/tap/rebang
```

安装脚本会校验 sha256，默认装到 `~/.local/bin`（Windows 为 `%LOCALAPPDATA%\Programs\rebang` 并加入用户 PATH）。可用环境变量调整：

| 变量 | 说明 |
| --- | --- |
| `REBANG_INSTALL_DIR` | 安装目录 |
| `REBANG_VERSION` | 安装指定版本，如 `0.1.0` |
| `REBANG_INSTALL_SKILL` | `1` 同时安装 Agent Skill，`0` 不安装 |

检测到本机装有 Claude Code、Codex（`~/.claude`、`~/.codex`、`~/.agents`）时，安装脚本会询问是否同时安装 rebang Agent Skill，直接回车即安装；没有终端可询问时（CI 等）不安装。也可以用参数明确选择：

```bash
curl -fsSL https://cli.rebang.today/install.sh | sh -s -- --with-skill   # 或 --no-skill
```

安装后不需要登录即可使用热榜、搜索与 AI 总结；我的关注、内容监控等功能需要时再运行 `rebang auth login`。

也可以从 [GitHub Releases](https://github.com/rebang-today/rebang-cli/releases) 手动下载，或从源码安装（需要 Go 1.26+）：

```bash
go install github.com/rebang-today/rebang-cli/cmd/rebang@latest
```

### macOS 提示“无法验证开发者”

程序没有做 Apple 签名。通过安装脚本或 Homebrew 安装不受影响；如果是用浏览器下载的安装包，解压后执行一次：

```bash
xattr -d com.apple.quarantine ./rebang
```

或者在 Finder 中右键程序选择“打开”，再到“系统设置 → 隐私与安全性”中点“仍要打开”。

## 快速开始

```bash
rebang                      # 进入交互界面
rebang auth login           # 可选：登录后可用我的关注、内容监控等（SSH 等无桌面环境加 --no-browser）
rebang list --tab zhihu     # 知乎热榜
rebang search 大模型        # 搜索
rebang summary <item_key>   # 条目的 AI 总结（item_key 来自 list -o json）
rebang monitor hits --unread # 内容监控中未读的命中内容（需登录）
rebang skill install        # 把 Skill 装进本机的 Agent
```

## 交互界面

| 按键 | 作用 |
| --- | --- |
| `↑` `↓` / `j` `k` | 在当前栏上下移动；在左栏移动时右栏榜单跟着切换 |
| `←` / `h` | 去左栏选来源（单栏时打开整栏的来源列表） |
| `→` / `l` | 回到榜单；在左栏按回车同样 |
| `tab` / `Shift+Tab` | 切换子榜单，到头后回到另一端 |
| `0`～`8`、`[` `]` | 切换分类（`0` 为订阅），每个分类记住上次的位置，下次启动自动回到上次的分类、榜单与子榜单 |
| `回车` | 榜单中用浏览器打开 |
| `Esc` | 退出搜索、帮助、搜索结果与整栏预览 |
| `/` | 搜索 |
| 空格 | 预览向下翻一屏，到底后回到开头 |
| `p` | 宽屏时打开或关闭预览；单栏时切换整栏预览（`p` 或 `Esc` 返回） |
| `y` | 复制链接 |
| `r` | 刷新 |
| `u` | 内容监控中切换全部、只看未读 |
| `g` `G` / `PgUp` `PgDn` | 开头、结尾、翻页 |
| `?` | 帮助 |
| `q` | 退出 |

顶部是分类栏，最前面的“订阅”列出登录用户订阅的榜单（其中的“内容监控”显示命中内容：回车打开原文并标为已读，`u` 只看未读；监控计划在网站上管理）；左栏是当前分类的榜单，右栏上方是子榜单条（只有一个子榜单时不显示）。标题等外部文本中的不可见字符会被去掉，`-o json` 保留原文。窗口较窄时底部先缩短账号状态，再按重要性省略快捷键提示，`←` 来源与 `→` 进入榜单始终保留。支持鼠标：点击分类、榜单、子榜单切换，点击条目选中、双击打开，滚轮滚动指针所在的区域（榜单、预览、菜单栏、子榜单条）而不改变选中项，支持的终端（kitty、Ghostty、foot 等）中悬停可点击元素时指针变为小手。窗口较窄（56 列以下）时切换为单栏，默认显示榜单，按 `←` 打开来源列表（`↑` `↓` 即切换，`→` 或回车回到榜单）；最小尺寸 40×10，更小时提示放大窗口；主题跟随终端深浅色，配色以网站蓝为主（深色终端用更柔和的蓝），前 3 名序号为橙色。上次浏览的位置保存在配置目录的 `tui-state.json`，下次启动自动恢复。

预览区显示选中条目的来源、时间、热度、AI 总结（一句话总结，以及要点、经过、观点与网友怎么看）和简介，内容较长时最后一行显示“↓ 更多”，按空格翻页。双栏时预览在榜单下方（窗口至少 24 行才显示），单栏时按 `p` 切换整栏预览；`p` 开关会被记住。榜单按配置的间隔自动刷新（默认 5 分钟）。

## 命令

| 命令 | 说明 |
| --- | --- |
| `menus` | 菜单列表 |
| `tabs --menu <id\|key\|名称>` | 菜单下的 Tab 与二级 Tab |
| `nodes --menu <…>` / `nodes --tab <key>` | 节点（榜单）列表 |
| `list --node <id>` / `list --tab <key> [--sub-tab <key>]` | 榜单，`-n` 指定条数（默认 30） |
| `search <关键词> [--days N] [--source key] [--sort latest] [-n N]` | 搜索，非会员只能搜当天 |
| `summary <item_key>` | 条目的 AI 总结；item_key 来自 `list`、`search` 的 JSON |
| `following` | 我的关注（需登录） |
| `monitor plans` / `monitor hits [--plan <ID>] [--unread] [-n N]` | 内容监控的计划与命中内容（需登录，只读） |
| `open <链接或序号> [--node/--tab …] [--print] [--mobile]` | 用浏览器打开 |
| `auth login \| logout \| status` | 登录、退出、查看状态 |
| `account` | 账号与会员状态 |
| `skill install \| uninstall \| status` | 管理 Agent Skill |
| `update [--check]` | 升级 |
| `doctor` | 环境诊断 |
| `version` | 版本信息 |

每个命令的详细参数见 `rebang <命令> --help`。

### 输出格式

查询类命令支持 `-o json|md|table`。输出到终端时默认 `table`，被管道或 Agent 读取时默认 `json`。结果写标准输出，提示与进度写标准错误，管道里的 JSON 始终可以直接解析：

```bash
rebang list --tab weibo -n 10 | jq -r '.items[] | "\(.rank). \(.title)"'
```

查看某条的 AI 总结：先从榜单 JSON 中取 `item_key`，再调用 `summary`。没有总结时退出码为 8：

```bash
key=$(rebang list --tab zhihu -n 1 -o json | jq -r '.items[0].item_key')
rebang summary "$key"           # 可读文本
rebang summary "$key" -o json   # {"summary_short": "...", "detail": {...}}
```

定时拉取内容监控的新命中（只读，不会标为已读），例如交给 Agent 每小时汇总一次：

```bash
rebang monitor plans -o json                       # 计划、关键词、状态与未读数
rebang monitor hits --unread -n 50 -o json | jq -r '.items[] | "\(.title) \(.url)"'
```

### 错误与退出码

出错时标准错误输出结构化信息（JSON 模式下为 `{"error":{"code","message","hint"}}`），退出码固定：

| 退出码 | 含义 | `code` 示例 |
| --- | --- | --- |
| 0 | 成功 | |
| 1 | 一般错误 | `general_error`、`not_found`、`forbidden` |
| 2 | 参数错误 | `invalid_argument` |
| 3 | 未登录或凭证失效 | `unauthorized` |
| 4 | 需要会员 | `membership_required` |
| 5 | 被限流 | `rate_limited` |
| 6 | 网络错误 | `network_error`、`server_error` |
| 7 | 需要升级 | `upgrade_required` |
| 8 | 条目暂无 AI 总结（`summary`） | `summary_not_found` |

网络错误与服务端 5xx 不会清除本地凭证，只有服务端明确判定凭证失效时才需要重新登录。

## 登录与会员

登录是可选的：热榜、搜索与 AI 总结都不需要登录，我的关注、内容监控等功能需要时再登录。

`rebang auth login` 使用浏览器设备授权：终端显示授权码并打开浏览器，在网站登录并确认后即完成，网站支持的所有登录方式都可以使用。凭证保存在配置目录的 `auth.json`（权限 0600），不使用系统钥匙串。

非终端或 `-o json` 时，拿到授权码后会立即在标准错误输出一行 JSON：`{"event":"login_pending","verification_uri":…,"verification_uri_complete":…,"user_code":…,"expires_in":…}`，Agent 可以把链接与授权码转告用户，命令在用户确认后自行退出。

在 CI 或 Agent 中也可以直接注入 Token，优先级高于本地凭证：

```bash
export REBANG_TOKEN=...
```

会员身份由服务端判断，`rebang account` 查看会员状态与到期时间，购买与续费请前往 [rebang.today/pricing](https://rebang.today/pricing)。

## Agent Skill

```bash
rebang skill install              # 安装到本机已有的 Agent（Claude Code、Codex 等）
rebang skill install --agent claude
rebang skill status
rebang skill uninstall
```

Skill 内嵌在程序中，`rebang update` 升级后会自动同步已安装的 Skill，保证与程序版本一致。

### 交给 Agent 的安装指引

想让 Claude Code、Codex 等 Agent 自己完成安装，可以把下面这段发给它：

```text
请帮我安装今日热榜命令行 rebang，并把它的 Agent Skill 装到本机：
1. 运行 curl -fsSL https://cli.rebang.today/install.sh | sh -s -- --with-skill
   （Windows 用 PowerShell：$env:REBANG_INSTALL_SKILL=1; irm https://cli.rebang.today/install.ps1 | iex）
2. 运行 rebang skill install，确认 Skill 已安装（rebang skill status）。
3. 运行 rebang list --tab zhihu -n 5 -o json 验证可用，这一步不需要登录。
不需要登录。以后用到我的关注、内容监控等功能时，再按需帮我登录。
```

安装脚本装在 `~/.local/bin`，如果 Agent 提示找不到 `rebang`，可以用完整路径 `~/.local/bin/rebang` 运行。

Agent 通过 JSON 调用时，如果有新版本或已安装的 Skill 与程序版本不一致，结果的顶层对象会附带 `_notice` 字段（`update`、`skill` 各带一条建议命令），Skill 会让 Agent 在完成任务后提醒你，不会自行升级。设置 `REBANG_NO_NOTICE=1` 可关闭。

## 配置

配置文件位于 `~/.config/rebang/config.toml`（Windows 为 `%AppData%\rebang\config.toml`），所有项均可省略：

```toml
output = "table"          # 默认输出格式：json、md、table，留空则按终端自动选择
auto_update = false       # 交互界面发现新版本时是否自动升级
refresh_interval = "5m"   # 交互界面自动刷新间隔，"0" 关闭，最短 30s
theme = "auto"            # auto、dark、light
```

环境变量：

| 变量 | 说明 |
| --- | --- |
| `REBANG_TOKEN` | 访问凭证，优先于本地登录 |
| `REBANG_CONFIG_DIR` | 配置目录 |
| `REBANG_API_BASE` | 接口地址，用于本地联调 |
| `REBANG_NO_NOTICE` | 设为 `1` 时 JSON 结果不附带 `_notice` 版本提醒 |

## 升级

```bash
rebang update           # 升级到最新版本
rebang update --check   # 只检查
```

升级包下载后会校验 sha256 与 minisign 签名。通过 Homebrew 安装的请使用 `brew upgrade rebang`。交互界面每天最多检查一次新版本并在状态栏提示；JSON 输出也共用这个每天一次的检查（最多等 1.5 秒，失败静默），发现新版本时只在结果的 `_notice` 字段中提示，不输出其他内容。

## 开发

```bash
make check      # gofmt、go vet、go test
make build      # 构建到 bin/rebang
make snapshot   # 本地试跑发布流程（需要 goreleaser）
```

需求与决策记录见 [docs/requirements.md](docs/requirements.md)，开发约定见 [AGENTS.md](AGENTS.md)。发版流程见 [docs/release.md](docs/release.md)。

## 许可证

[Apache-2.0](LICENSE)
