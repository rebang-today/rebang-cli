# AI 总结：summary

部分热榜条目有 AI 根据原文生成的总结，包括简述、要点、事件经过、各方观点与网友反应。不需要登录。

```bash
rebang list --tab zhihu -n 10 -o json   # 取 items[].item_key
rebang summary <item_key> -o json
```

| 参数 | 说明 |
| --- | --- |
| `<item_key>` | 必填，32 位，来自 `list`、`search`、`monitor hits` 结果中的 `item_key` |

## 输出

- `summary_short`：简述，一到三个短句，通常用一处 `**…**` 标出核心结论。
- `detail`：分区都可能缺失，不认识的分区忽略：
  - `points`：要点，字符串数组。
  - `timeline`：经过，`[{time, text}]`。
  - `viewpoints`：观点，`[{stance, text, by, quote}]`，`quote` 为原话。
  - `reactions`：网友怎么看，`{mood, voices: [{by, text, quote}]}`。

文本中的 `**…**` 是强调标记，转述时可以保留为加粗。

## 注意

- 没有总结时退出码为 8，这不是错误，不要重试；改用标题、`desc` 与原链接。
- 只对用户关心的少数条目调用，不要对整页榜单逐条查询。
- 总结是 AI 生成的，引用时说明“据今日热榜的 AI 总结”并附原链接；需要准确细节（数字、原话）时以原文为准。
