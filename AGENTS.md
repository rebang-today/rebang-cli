# AGENTS.md

本仓库是今日热榜（rebang.today）的官方命令行客户端 `rebang`，Go 编写，开源（Apache-2.0）。

## 必读

- 需求与决策记录：`docs/requirements.md`（决策编号 D-xx）。所有行为以它为准，改动涉及决策时同步更新该文档。
- 服务端是私有仓库，客户端不包含任何服务端代码；请求与响应结构在 `internal/api` 中自行定义（D-03）。

## 目录

| 路径 | 说明 |
| --- | --- |
| `cmd/rebang` | 程序入口 |
| `internal/cli` | cobra 子命令（非交互调用，D-30） |
| `internal/tui` | Bubble Tea 交互界面（D-20 ~ D-24） |
| `internal/api` | `/v1` 接口客户端、错误映射 |
| `internal/auth` | 设备授权登录（RFC 8628） |
| `internal/credentials` | `auth.json` 凭证（0600、原子写入） |
| `internal/config` | `config.toml` 与配置目录 |
| `internal/output` | json / md / table 输出与错误输出 |
| `internal/errs` | 结构化错误与退出码（D-32） |
| `internal/update` | manifest、sha256、minisign 校验与自升级 |
| `internal/skill` | 内嵌 Agent Skill（`files/` 目录即 skill 内容） |
| `internal/release`、`tools/manifestgen` | 发版时生成 `manifest.json`，不随程序发布 |
| `scripts/` | `install.sh`、`install.ps1`，发版时上传到 CDN |
| `site/` | 下载站首页 `index.html`（安装教程），发版时上传到 cli.rebang.today 根目录；内容与 README 保持一致，不引用外部资源 |
| `.goreleaser.yaml`、`.github/workflows/` | 构建、签名、发布流水线，说明见 `docs/release.md` |

安装包命名 `rebang_<版本>_<系统>_<架构>.tar.gz|zip` 被 goreleaser、manifestgen、安装脚本与 `internal/update` 共同依赖，修改时需同步。`internal/update/minisign.pub` 是发布公钥，只能替换为正式公钥，不要提交测试密钥。

## 约定

- 命令名、参数名、JSON 字段、错误码用英文；界面文案用中文（D-05）。
- JSON 输出字段与退出码在 1.0 之后是稳定接口，新增可以，改名和删除需要升主版本（D-56）。
- 结果写标准输出，提示与进度写标准错误，保证管道里的 JSON 可解析。
- 只有服务端明确返回 401 时才提示重新登录；网络错误与 5xx 不得清除凭证（D-34）。
- 请求一律通过 `internal/api.Client`，保证 User-Agent 为 `rebang-cli/<版本> (<系统>/<架构>)`（D-33）。
- 不提交服务器地址、内部域名、密钥和后台接口。默认地址只能是公开地址，联调用环境变量覆盖。
- 修改 skill 内容时编辑 `internal/skill/files/`，版本号用 `{{VERSION}}` 占位。

## 开发

```bash
make check          # gofmt 检查、go vet、go test
make snapshot       # 本地试跑 goreleaser（不签名、不发布）
make build          # 构建到 bin/rebang
REBANG_API_BASE=http://127.0.0.1:9898 ./bin/rebang menus   # 连本地服务端
REBANG_CONFIG_DIR=$(mktemp -d) ./bin/rebang auth status     # 隔离配置目录
```

测试用 `httptest` 模拟服务端，不访问网络。新增命令时在 `internal/cli/*_test.go` 中补充用例，至少覆盖 JSON 输出与退出码。

提交信息使用 Conventional Commits（`feat:`、`fix:`、`docs:`、`chore:` 等）。
