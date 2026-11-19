# 搜索热榜条目：search

按关键词搜索上过热榜的条目，回答“这件事有没有上过热榜”“最近各平台在讨论什么”。

```bash
rebang search "大模型" -o json                              # 当天
rebang search "英伟达" --days 7 -n 50 -o json               # 最近 7 天（会员）
rebang search "英伟达" --source weibo,zhihu -o json         # 限定来源（会员）
rebang search "英伟达" --days 30 --sort latest -o json      # 按时间排序（会员）
```

| 参数 | 说明 |
| --- | --- |
| `<关键词>` | 必填。多个词会用空格拼成一个查询 |
| `--days <N>` | 搜索最近 N 天，1~365，默认当天 |
| `--source <KEY>` | 限定来源 Tab KEY，可重复或用逗号分隔；KEY 来自 `rebang tabs --all` |
| `--sort <方式>` | `relevance`（默认，按相关度）或 `latest`（按时间） |
| `-n, --limit <N>` | 最多返回条数，1~200，默认 20 |

## 会员限制

普通用户只能搜当天、全部来源、按相关度排序。`--days` 大于 1、`--source`、`--sort latest` 任一需要会员，否则退出码为 4。遇到时：

1. 告诉用户“搜索更早的时间（或限定平台、按时间排序）需要今日热榜会员”。
2. 去掉受限参数，在权限内完成（例如只搜当天，或用 `list --tab` 看对应平台的实时榜单），并说明结果因此受限。
3. 不要编造受限部分的结果。用户想开通时，引导到 `rebang account` 输出的 `manage_url`（需登录）或 https://rebang.today/pricing 。

## 输出

JSON：

- `query`、`time_range`、`start_date`、`end_date`、`sources`、`sort`：本次实际使用的条件。
- `total`：命中总数；`has_more`：是否还有更多（增大 `-n` 或缩小条件）。
- `items[]`：`title`、`url`、`mobile_url`、`snippet`（命中片段）、`source`（Tab KEY）、`source_name`（平台名）、`item_key`、`first_seen_at`（首次上榜）、`last_seen_at`（最后在榜）。

## 写好关键词

- 关键词短一点（1~3 个词），按词匹配，长句效果差。
- 一个主体多试几种叫法：中文名、英文名、简称、相关人物或产品。例如“英伟达”“NVIDIA”“黄仁勋”。
- 结果可能包含同名或顺带提及的无关条目，引用前检查标题与 `snippet` 是否真的在讲这件事。
- 需要多次搜索并汇总成报告时，使用 `rebang-research` skill。
