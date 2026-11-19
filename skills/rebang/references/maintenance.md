# 安装、升级与诊断：skill、update、doctor、version

这些命令维护 rebang 本身。除 `version`、`doctor`、`skill status` 外，都只在用户要求时运行。

## rebang version

JSON：`version`、`commit`、`date`、`platform`、`go_version`。

## rebang doctor

环境诊断：接口连通、凭证、版本检查、Agent Skill 等。命令失败原因不明时运行一次，把 `status` 为 `fail` 或 `warn` 的项告诉用户。

JSON：`ok`（没有失败项）与 `checks[]`：`name`、`status`（`ok`、`warn`、`fail`）、`detail`。有失败项时退出码为 1。

## rebang update

```bash
rebang update --check -o json   # 只检查是否有新版本
rebang update                   # 升级（用户同意后）
```

| 参数 | 说明 |
| --- | --- |
| `--check` | 只检查，不升级 |
| `--force` | 已是最新版本也重新安装 |

升级会校验 sha256 与 minisign 签名，并同步已安装的 skill。通过 Homebrew 安装时改用 `brew upgrade rebang`。

JSON：`current_version`、`latest_version`、`min_version`、`updated`、`check_only`、`notes_url`、`skills_updated`。

## rebang skill

管理随程序发布的 skill：`rebang`（本手册）与 `rebang-*` 应用，作为一组安装、升级。

```bash
rebang skill status -o json
rebang skill install                 # 安装到本机已有的 Agent
rebang skill install --agent claude
rebang skill uninstall
```

| 参数 | 说明 |
| --- | --- |
| `--agent <名称>` | `claude`、`codex`、`agents`、`cursor`、`gemini` 或 `all`，可重复；默认本机已有的 Agent |
| `--dir <目录>` | 自定义 skill 根目录 |
| `--force` | 覆盖不是由 rebang 安装的同名目录（仅 `install`），先征得用户同意 |

`skill status` 的 JSON：`version` 与 `skills[]`：`agent`、`dir`、`installed`、`version`、`skills`（已安装的 skill 名称）、`up_to_date`、`foreign`（同名目录不是由 rebang 安装的，例如通过 `npx skills` 安装）。

## 环境变量

| 变量 | 说明 |
| --- | --- |
| `REBANG_TOKEN` | 访问凭证，优先于本地登录 |
| `REBANG_NO_NOTICE` | 设为 `1` 时 JSON 结果不附带 `_notice` |
| `REBANG_CONFIG_DIR` | 配置目录 |
| `REBANG_API_BASE` | 接口地址，仅用于开发联调 |
