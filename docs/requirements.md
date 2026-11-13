# rebang CLI 需求与决策记录

> 状态：草案，待评审。本文档位于开源仓库，不包含服务器地址、内部域名和后台接口。服务端实现细节见服务端仓库的 CLI 认证文档，两边只通过本文的决策编号（D-xx）互相引用。

## 1. 目标

`rebang` 是今日热榜（rebang.today）的官方命令行客户端，同时服务两类使用者：

- **人**：在终端里用键盘和鼠标浏览热榜，体验接近网站。
- **AI Agent 与脚本**：通过子命令拿到结构化结果，配合随版本发布的 Agent Skill 完成查询与研究任务。

用户登录后享有与网站一致的账号与会员能力。

## 2. 非目标（第一版不做）

- CLI 内购买、激活或续费会员（跳转网页完成）。
- 设置编辑、监控计划、反馈、表情回应、推荐返利。
- 客户端埋点遥测。
- macOS 代码签名与公证。
- 英文、日文界面（结构上预留）。
- 专门的接口契约文档（需要时直接阅读服务端代码）。

## 3. 决策

### 形态与技术

- **D-01 单一程序两种用法**：`rebang` 无参数进入全屏交互界面（TUI）；`rebang <子命令>` 为非交互调用。
- **D-02 技术栈**：Go + Bubble Tea（Lip Gloss、Bubbles）。模块路径 `github.com/rebang-today/rebang-cli`，命令名 `rebang`。
- **D-03 独立开源仓库**：`rebang-today/rebang-cli`，许可证 Apache-2.0。客户端不包含任何服务端代码，请求与响应结构在本仓库自行定义。
- **D-04 支持平台**：macOS、Linux、Windows，各自 amd64 与 arm64。
- **D-05 界面语言**：第一版仅中文；命令名、参数名、JSON 字段、错误码一律英文，保证 Agent 调用稳定。

### 账号与会员

- **D-10 登录方式**：浏览器设备授权（RFC 8628）。CLI 展示授权码并打开浏览器，用户在网站登录并确认后，CLI 获得专属 Token。网站已有的全部登录方式（邮箱验证码、密码 + 2FA、第三方登录）自动可用。支持 `--no-browser`，适配 SSH 等无桌面环境。
- **D-11 环境变量**：`REBANG_TOKEN` 优先级最高，供 CI 与 Agent 注入凭证。
- **D-12 凭证存储**：统一存放在文件中，不接入系统钥匙串。
  - macOS/Linux：`~/.config/rebang/auth.json`
  - Windows：`%AppData%\rebang\auth.json`
  - 文件权限 0600，原子写入（临时文件 + 重命名），文件格式带版本号。
- **D-13 Token 有效期**：90 天滑动续期，期间使用即自动顺延。CLI Token 独立于网页登录的会话上限，单个用户最多 10 台设备，可在网站设置中按设备吊销。
- **D-14 会员**：CLI 识别会员身份并享有与网站一致的会员能力（例如搜索时间窗口），所有会员判断由服务端完成，客户端不做额外限制。`rebang account` 展示会员状态与到期时间；购买与续费引导到网页。

### 交互界面（TUI）

- **D-20 第一版功能**：菜单 → Tab → 榜单三级浏览、回车用浏览器打开、复制链接、搜索、我的关注、切换节点、手动与定时刷新。
- **D-21 布局**：左栏菜单与 Tab 树，右栏榜单，底部状态栏显示登录与会员状态和快捷键提示；窄窗口自动切换为单栏。
- **D-22 键盘**：方向键与 `hjkl` 导航，`/` 搜索，回车打开，`y` 复制链接，`r` 刷新，`?` 帮助，`q` 退出。
- **D-23 鼠标**：点击选中、滚轮滚动、双击打开。
- **D-24 主题**：自动跟随终端深色或浅色。

### 非交互命令

- **D-30 第一版命令集**

  | 命令 | 说明 |
  | --- | --- |
  | `auth login \| logout \| status` | 登录、退出、查看状态 |
  | `account` | 账号与会员状态 |
  | `menus` | 菜单列表 |
  | `tabs --menu <id>` | 指定菜单下的 Tab |
  | `nodes --menu <id> \| --tab <id>` | 节点列表 |
  | `list --node <id>` | 榜单 |
  | `search <关键词> [--days N] [--source …] [--limit N]` | 搜索 |
  | `following` | 我的关注 |
  | `open <条目>` | 用浏览器打开 |
  | `skill install \| uninstall \| status` | 管理 Agent Skill |
  | `update` | 升级 |
  | `doctor` | 环境诊断 |
  | `version` | 版本信息 |

- **D-31 输出格式**：查询类命令支持 `-o json|md|table`。输出到终端时默认 `table`，被管道或 Agent 调用时默认 `json`。
- **D-32 错误与退出码**：错误统一输出结构化 JSON（`code`、`message`、`hint`）。退出码固定：0 成功、1 一般错误、2 参数错误、3 未登录或凭证失效、4 需要会员、5 被限流、6 网络错误、7 需要升级。
- **D-33 请求标识**：每个请求携带 `User-Agent: rebang-cli/<版本> (<系统>/<架构>)`，非交互调用另带调用方式标记，供服务端区分人与 Agent 流量。UA 中不得包含 `go-http-client` 等通用库标识。
- **D-34 网络错误不清凭证**：只有服务端明确判定 Token 无效、过期或已吊销时，才提示重新登录；网络错误、超时、5xx 一律不清除本地凭证。

### 配置

- **D-40 配置文件**：macOS/Linux `~/.config/rebang/config.toml`，Windows `%AppData%\rebang\config.toml`。配置项：默认输出格式、`auto_update`、刷新间隔、主题。

### 升级与发布

- **D-50 升级策略**：
  - `rebang update` 手动升级。
  - 交互使用时每天最多检查一次新版本并提示；非交互调用时不检查、不输出提示。
  - 默认不静默替换程序，可通过 `auto_update` 开启。
  - 版本低于 `min_version` 时拒绝执行并提示升级（退出码 7）。
- **D-51 版本信息文件**：自有 CDN 上的静态文件 `cli/manifest.json`，包含最新版本、`min_version`、各平台下载地址与 sha256。升级只读取这个文件；需要强制升级时手动修改 `min_version`。
- **D-52 完整性校验**：下载后校验 sha256，并用 minisign 验证签名，公钥内置在程序中。
- **D-53 分发渠道**：自有 CDN 为主（国内下载快），GitHub Releases 与 Homebrew tap（`rebang-today/homebrew-tap`）为辅。
- **D-54 安装方式**：
  - macOS/Linux：`curl -fsSL <官网>/cli/install.sh | sh`
  - Windows：`irm <官网>/cli/install.ps1 | iex`
  - macOS 另可 `brew install rebang-today/tap/rebang`
  - 不提供 npm 与 scoop。
- **D-55 发版流水线**：推送 `v*` tag 触发 GitHub Actions，由 goreleaser 依次完成：构建、生成 sha256 与 minisign 签名、发布 GitHub Releases、更新 Homebrew tap、上传 CDN，**最后**写入 `manifest.json`。需要的 Secrets：仅限 `cli/` 目录写权限的 OSS 子账号密钥、minisign 私钥、tap 仓库写权限 token。
- **D-56 版本号**：语义化版本，从 v0.1.0 开始。1.0 之前允许不兼容变更；1.0 之后命令名、参数、JSON 字段、退出码均视为稳定接口，不兼容变更必须升主版本。每个版本附带更新日志。

### Agent Skill

- **D-60 内嵌并随版本发布**：skill 文件编译进程序，`rebang skill install` 写入各 Agent 的 skill 目录（如 `~/.claude/skills/rebang`）。`rebang update` 升级后自动同步已安装的 skill，保证 skill 与程序版本一致。skill 中注明适用的版本范围。
- **D-61 结构**：一个名为 `rebang` 的 skill。`SKILL.md` 为命令用法，`references/research.md` 为研究流程。
- **D-62 研究流程**：
  1. 识别主体和同义词，拆成多条子查询。
  2. 多次调用 `rebang search` 检索。
  3. 按链接去重，检查结果是否真的在讲用户问的主体，再按热度排序。
  4. 综合作答，每条结论附原链接；证据不足时如实说明。
  5. 遇到退出码 4（需要会员，例如普通用户只能搜当天），如实告知用户，不得编造结果。

### 隐私与分发合规

- **D-70 不做客户端遥测**，统计依赖服务端日志中的 User-Agent。
- **D-71 不做 macOS 签名**：curl 与 Homebrew 安装不受 Gatekeeper 隔离影响；README 说明浏览器手动下载时如何放行。

## 4. 里程碑

| 里程碑 | 仓库 | 内容 | 依赖 |
| --- | --- | --- | --- |
| M1 后端 | 服务端 | 设备授权、CLI Token 存储与认证、按 Token 限流、设备列表与吊销 | 无 |
| M2 网页 | 网站前端 | 授权确认页、设置中的设备管理页 | 与 M1 并行，联调依赖 M1 |
| M3 CLI | 本仓库 | 子命令、登录、升级、skill → 交互界面 → 发布流水线，发布 v0.1.0 | 登录部分依赖 M1 |

## 5. 待办与开放问题

- 官网安装脚本与 `manifest.json` 的最终路径，在搭建 CDN 目录时确定。
- 定时刷新的默认间隔，在交互界面实现时确定。
