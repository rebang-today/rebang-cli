# 内容监控：monitor

用户在网站上创建监控计划（一组关键词），今日热榜持续收集命中的热榜内容。命令行只读：不会把命中标为已读，也不能创建或修改计划；需要改计划时请用户到网站操作。需要登录。

## rebang monitor plans

```bash
rebang monitor plans -o json
```

JSON：`unread_all`（全部未读数）与 `plans[]`：`id`、`name`、`keywords`、`status`、`unread_count`。

`status`：`active` 正常；`paused` 用户已暂停；`membership_expired` 会员到期后计划停止收集，如实告诉用户。

## rebang monitor hits

```bash
rebang monitor hits --unread -o json            # 全部计划的未读命中
rebang monitor hits --plan 12 -n 50 -o json     # 某个计划
```

| 参数 | 说明 |
| --- | --- |
| `--plan <ID>` | 只看某个计划，ID 来自 `monitor plans` |
| `--unread` | 只看未读 |
| `-n, --limit <N>` | 最多返回条数，1~100，默认 30 |

JSON：`plan_id`、`unread_only`、`has_more`，以及 `items[]`：`item_key`、`title`、`description`、`source`、`source_name`、`url`、`mobile_url`、`matched_at`（命中时间）、`published_at`、`is_read`、`matches[]`（`plan_id`、`plan_name`、`matched_keywords`）。

## 用法建议

- 定时汇总：用 `--unread` 拉取，按计划分组，每条附原链接；需要时对少数重要条目调用 `summary`。
- 因为命令行不会标记已读，同一条可能在下次拉取时再次出现。需要增量时按 `item_key` 或 `matched_at` 自行去重。
- `has_more` 为 true 时说明还有更多，不要声称已列出全部。
