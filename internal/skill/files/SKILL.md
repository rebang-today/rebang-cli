---
name: rebang
description: 查询今日热榜（rebang.today）的实时热榜与历史热点。当用户想知道某个平台（微博、知乎、抖音、B 站、GitHub 等）现在的热门内容、某个话题最近是否上过热榜、某件事在各平台的讨论热度，或需要基于热榜做舆情与话题研究时使用。通过本机安装的 rebang 命令行获取结构化结果。
---

# rebang：今日热榜命令行

适用版本：rebang {{VERSION}}。本 skill 随 rebang 程序一起发布，`rebang update` 升级后会自动同步；如果 `rebang version` 显示的版本与上面不一致，运行 `rebang skill install` 重新安装。

## 调用约定

- 在 Agent 中调用时输出默认为 JSON（标准输出不是终端）。需要时显式加 `-o json`；需要给用户看的列表可用 `-o md`。
- 结果写在标准输出；错误写在标准错误，格式为 `{"error":{"code":"…","message":"…","hint":"…"}}`。
- 根据退出码判断结果，不要解析错误文案：

| 退出码 | code | 含义与处理 |
| --- | --- | --- |
| 0 | | 成功 |
| 1 | `general_error` 等 | 一般错误，把 message 告诉用户 |
| 2 | `invalid_argument` | 参数错误，检查命令与 ID |
| 3 | `unauthorized` | 未登录或凭证失效：请用户在终端运行 `rebang auth login`，不要替用户登录 |
| 4 | `membership_required` | 需要会员：如实告知用户，不要编造结果 |
| 5 | `rate_limited` | 被限流：稍等再试，减少调用次数 |
| 6 | `network_error` / `server_error` | 网络或服务端错误：可重试一次 |
| 7 | `upgrade_required` | 版本过低：请用户运行 `rebang update` |
| 8 | `summary_not_found` | 该条目暂无 AI 总结：不是错误，改用标题、`desc` 或原链接，不要编造总结 |

## 浏览热榜

热榜按 菜单 → Tab → 节点（榜单）三级组织。

```bash
rebang menus                         # 菜单列表：id、menu_key、menu_name
rebang tabs --menu <菜单ID或KEY>      # 菜单下的 Tab，child 为二级 Tab
rebang nodes --menu <菜单ID或KEY>     # 菜单下的节点：node_id、name
rebang nodes --tab <TAB_KEY>          # 一级 Tab 下的二级 Tab
rebang list --node <NODE_ID> -n 20    # 某个节点的榜单
rebang list --tab weibo -n 20         # 也可直接用 Tab（可加 --sub-tab）
```

`list` 的 JSON：`items[]` 中每项有 `rank`、`title`、`url`、`heat`（可能为空）、`desc`、`published_at`、`item_key`。
`content_access.mode` 为 `preview` 表示非会员只能看到前 `preview_limit` 条。

不知道 Tab KEY 时，先 `rebang menus`，再 `rebang tabs --menu <id>`。

## 搜索

```bash
rebang search "关键词"                       # 默认：普通用户当天，会员最近一年
rebang search "关键词" --days 7 -n 50        # 最近 7 天（会员）
rebang search "关键词" --source weibo,zhihu  # 限定来源 Tab KEY（会员）
rebang search "关键词" --sort latest         # 按时间排序（会员）
```

JSON：`total`、`has_more`、`items[]`（`title`、`url`、`snippet`、`source`、`source_name`、`item_key`、`first_seen_at`、`last_seen_at`）。
`--limit` 最大 200。普通用户使用 `--days`（大于 1）、`--source`、`--sort latest` 会返回退出码 4。

## AI 总结

部分条目有 AI 生成的总结，不需要登录。先从 `list` 或 `search` 的 JSON 中取 `item_key`，再查询：

```bash
rebang list --tab zhihu -n 10 -o json          # 取 items[].item_key
rebang summary <item_key> -o json              # 单条的 AI 总结
```

JSON：`summary_short`（一句话总结）与 `detail`。`detail` 中的分区都可能缺失：`points`（要点，字符串数组）、`timeline`（经过，`[{time, text}]`）、`viewpoints`（观点，`[{stance, text, by, quote}]`）、`reactions`（网友怎么看，`{mood, voices: [{by, text, quote}]}`）；不认识的分区忽略。文本中的 `**…**` 是强调标记。

没有总结时退出码为 8，这不是错误，不要重试。总结是 AI 根据原文生成的，引用时说明来源并附原链接；需要准确细节时以原文为准。只对用户关心的少数条目调用，不要对整页逐条查询。

做话题研究、需要多次搜索并汇总时，按 [references/research.md](references/research.md) 的流程进行。

## 账号

```bash
rebang auth status    # 是否登录；未登录退出码 3
rebang account        # 账号与会员状态：is_member、member_expires_at
rebang following      # 我的关注：nodes[] 与 tabs[]（需登录）
```

### 内容监控（需登录，只读）

用户在网站上创建的监控计划会收集命中关键词的内容。命令行只读取，不会标为已读；创建、修改计划请让用户到网站操作。

```bash
rebang monitor plans -o json                 # unread_all 与 plans[]：id、name、keywords、status（active/paused/membership_expired）、unread_count
rebang monitor hits --unread -o json         # 定时拉取未读命中
rebang monitor hits --plan <ID> -n 50 -o json
```

hits 的 JSON：`items[]` 含 `item_key`、`title`、`source_name`、`url`、`matched_at`、`is_read`、`matches[]`（`plan_name`、`matched_keywords`），`has_more` 表示还有更多（`-n` 最大 100）。需要 AI 总结时用 `item_key` 调用 `summary`。

凭证可以通过环境变量 `REBANG_TOKEN` 注入。不要读取、打印或保存凭证文件内容。

## 其他

```bash
rebang open <链接>                 # 用浏览器打开
rebang open 3 --node <ID> --print  # 只输出榜单第 3 条的链接
rebang doctor                      # 环境诊断
rebang version
```

## 注意

- 引用热榜内容时附上原链接（`url`），不要编造标题、热度或链接。
- 热榜实时变化，回答中注明查询时间。
- 不要在循环中高频调用；同一查询结果在几分钟内可以复用。
