# 登录、账号与我的关注：auth、account、following

热榜、搜索、AI 总结都不需要登录。我的关注、内容监控、账号信息需要登录。登录一律按需进行。

## 按需登录

只有命令以退出码 3 结束时，才告诉用户“这个功能需要登录今日热榜”，并询问是否现在登录。不要事先要求登录，也不要在用户没同意时发起登录。

用户同意后：

1. 在后台运行 `rebang auth login --no-browser`。它会立刻在标准错误输出链接和授权码，以及一行 JSON：`{"event":"login_pending","verification_uri":"…","verification_uri_complete":"…","user_code":"…","expires_in":600}`。
2. 把链接（优先 `verification_uri_complete`）和授权码原样转告用户，请用户在浏览器中打开链接、核对授权码并确认。不要改写或缩短链接。
3. 等命令自己退出，不要中途结束它。用户确认后以退出码 0 退出；用户拒绝或授权码过期（`expires_in` 秒）时以非 0 退出，告诉用户结果即可。
4. 运行 `rebang auth status` 确认已登录，再重试刚才的命令。

用户拒绝时，继续用不需要登录的功能完成任务，并说明哪些内容因未登录而缺失。

在用户自己的服务器、CI 等环境中，也可以由用户设置环境变量 `REBANG_TOKEN` 提供凭证，它优先于本地登录。不要读取、打印或保存凭证文件内容。

## rebang auth

```bash
rebang auth login --no-browser   # 设备授权登录，见上文
rebang auth status -o json       # 是否已登录；未登录退出码 3
rebang auth status --offline     # 只检查本地凭证，不访问服务端
rebang auth logout               # 退出登录，并让服务端吊销本机凭证
```

| 子命令与参数 | 说明 |
| --- | --- |
| `auth login --no-browser` | 不自动打开浏览器。由你转告链接时必须加 |
| `auth status --offline` | 不访问服务端，只看本地是否有凭证 |
| `auth logout` | 只在用户明确要求时运行 |

`auth status` 的 JSON：`logged_in`、`source`（凭证来源：本地文件或环境变量）、`token`（已打码）、`verified`（是否已向服务端验证）、`account`（同 `account` 命令）。

## rebang account

账号与会员状态，需要登录。

JSON：`id`、`email`、`email_bound`、`created_at`、`is_member`、`member_expires_at`、`manage_url`（网站上管理会员的地址）。

会员身份以这里的 `is_member` 为准。用户问“我是不是会员”“会员什么时候到期”时用它。

## rebang following

用户在网站上关注的榜单，需要登录。

JSON：`nodes[]`（关注的节点：`node_id`、`name`、`subtitle`…）与 `tabs[]`（关注的 Tab：`key`、`name`、`child`）。

用户说“看看我关注的榜单”时，先取 `following`，再对其中的节点或 Tab 调用 `list`。
